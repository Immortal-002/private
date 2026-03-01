/**
 * Distributed System Telemetry Agent
 * Collects system metrics and sends to backend; polls and executes remote commands.
 * Features:
 *   - Agent registration with hostname/OS
 *   - Telemetry collection every 5 seconds
 *   - Command polling every ~10 seconds
 *   - Retry with exponential backoff on network failure
 *   - Local file logging
 * Dependencies: libcurl only.
 */

#include <chrono>
#include <cstdlib>
#include <cstring>
#include <ctime>
#include <fstream>
#include <iostream>
#include <mutex>
#include <sstream>
#include <string>
#include <thread>
#include <vector>
#include <sys/statvfs.h>
#include <sys/utsname.h>
#include <unistd.h>

#include <curl/curl.h>

namespace {

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------
const char* API_URL = nullptr;
std::string AGENT_ID;
std::string AGENT_HOSTNAME;
std::string AGENT_OS;
const int TELEMETRY_INTERVAL_SEC = 5;
const int MAX_RETRY = 5;
const char* LOG_FILE_PATH = "agent.log";

// ---------------------------------------------------------------------------
// Logging (stdout + file)
// ---------------------------------------------------------------------------
std::mutex log_mutex;
std::ofstream log_file;

enum LogLevel { LOG_INFO, LOG_WARN, LOG_ERROR };

void log_msg(LogLevel level, const std::string& msg) {
    const char* tag = "INFO";
    if (level == LOG_WARN) tag = "WARN";
    else if (level == LOG_ERROR) tag = "ERROR";

    auto now = std::chrono::system_clock::now();
    auto t = std::chrono::system_clock::to_time_t(now);
    char timebuf[64];
    std::strftime(timebuf, sizeof(timebuf), "%Y-%m-%d %H:%M:%S", std::localtime(&t));

    std::lock_guard<std::mutex> lock(log_mutex);
    std::string line = std::string(timebuf) + " [" + tag + "] " + msg;
    std::cout << line << std::endl;
    if (log_file.is_open()) {
        log_file << line << std::endl;
        log_file.flush();
    }
}

// ---------------------------------------------------------------------------
// HTTP helpers with retry & exponential backoff
// ---------------------------------------------------------------------------
struct Response {
    std::string data;
    long http_code = 0;
};

size_t write_cb(void* ptr, size_t size, size_t nmemb, void* user) {
    auto* r = static_cast<Response*>(user);
    r->data.append(static_cast<char*>(ptr), size * nmemb);
    return size * nmemb;
}

Response http_request(const std::string& url, const std::string& method,
                      const std::string& body = "") {
    Response resp;
    int attempt = 0;
    int backoff_ms = 500;

    while (attempt < MAX_RETRY) {
        attempt++;
        resp.data.clear();
        resp.http_code = 0;

        CURL* curl = curl_easy_init();
        if (!curl) {
            log_msg(LOG_ERROR, "curl_easy_init failed");
            return resp;
        }

        struct curl_slist* headers = nullptr;
        headers = curl_slist_append(headers, "Content-Type: application/json");

        curl_easy_setopt(curl, CURLOPT_URL, url.c_str());
        curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
        curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, write_cb);
        curl_easy_setopt(curl, CURLOPT_WRITEDATA, &resp);
        curl_easy_setopt(curl, CURLOPT_TIMEOUT, 10L);
        curl_easy_setopt(curl, CURLOPT_CONNECTTIMEOUT, 5L);

        if (method == "POST") {
            curl_easy_setopt(curl, CURLOPT_POSTFIELDS, body.c_str());
        }

        CURLcode res = curl_easy_perform(curl);
        if (res == CURLE_OK) {
            curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &resp.http_code);
        }

        curl_slist_free_all(headers);
        curl_easy_cleanup(curl);

        if (res == CURLE_OK && resp.http_code >= 200 && resp.http_code < 500) {
            return resp;
        }

        log_msg(LOG_WARN, "HTTP " + method + " " + url + " failed (attempt " +
                          std::to_string(attempt) + "/" + std::to_string(MAX_RETRY) +
                          ") curl=" + std::to_string(res) +
                          " http=" + std::to_string(resp.http_code) +
                          " - retrying in " + std::to_string(backoff_ms) + "ms");

        std::this_thread::sleep_for(std::chrono::milliseconds(backoff_ms));
        backoff_ms = std::min(backoff_ms * 2, 16000);
    }

    log_msg(LOG_ERROR, "HTTP " + method + " " + url + " failed after " +
                       std::to_string(MAX_RETRY) + " retries");
    return resp;
}

Response http_get(const std::string& url) {
    return http_request(url, "GET");
}

Response http_post(const std::string& url, const std::string& body) {
    return http_request(url, "POST", body);
}

// ---------------------------------------------------------------------------
// JSON escape helper (no external JSON lib)
// ---------------------------------------------------------------------------
std::string escape_json(const std::string& s) {
    std::string out;
    for (char c : s) {
        if (c == '"') out += "\\\"";
        else if (c == '\\') out += "\\\\";
        else if (c == '\n') out += "\\n";
        else if (c == '\r') out += "\\r";
        else if (c == '\t') out += "\\t";
        else out += c;
    }
    return out;
}

// ---------------------------------------------------------------------------
// System info collection
// ---------------------------------------------------------------------------
double read_cpu_usage() {
    static unsigned long long prev_idle = 0, prev_total = 0;
    std::ifstream f("/proc/stat");
    if (!f) return 0.0;
    std::string line;
    if (!std::getline(f, line)) return 0.0;
    unsigned long long user = 0, nice = 0, system = 0, idle = 0;
    unsigned long long iowait = 0, irq = 0, softirq = 0, steal = 0;
    sscanf(line.c_str(), "cpu %llu %llu %llu %llu %llu %llu %llu %llu",
           &user, &nice, &system, &idle, &iowait, &irq, &softirq, &steal);
    unsigned long long total = user + nice + system + idle + iowait + irq + softirq + steal;
    double usage = 0.0;
    if (prev_total > 0 && total > prev_total) {
        unsigned long long diff_total = total - prev_total;
        unsigned long long diff_idle = (idle + iowait) - prev_idle;
        if (diff_total > 0)
            usage = 100.0 * (1.0 - static_cast<double>(diff_idle) / diff_total);
    }
    prev_idle = idle + iowait;
    prev_total = total;
    return usage;
}

double read_memory_usage() {
    std::ifstream f("/proc/meminfo");
    if (!f) return 0.0;
    unsigned long mem_total = 0, mem_avail = 0;
    std::string line;
    while (std::getline(f, line)) {
        if (line.find("MemTotal:") == 0) sscanf(line.c_str(), "MemTotal: %lu", &mem_total);
        else if (line.find("MemAvailable:") == 0) sscanf(line.c_str(), "MemAvailable: %lu", &mem_avail);
    }
    if (mem_total == 0) return 0.0;
    return 100.0 * (1.0 - static_cast<double>(mem_avail) / mem_total);
}

void read_memory_bytes(unsigned long& total_kb, unsigned long& used_kb) {
    std::ifstream f("/proc/meminfo");
    total_kb = used_kb = 0;
    if (!f) return;
    unsigned long mem_total = 0, mem_avail = 0;
    std::string line;
    while (std::getline(f, line)) {
        if (line.find("MemTotal:") == 0) sscanf(line.c_str(), "MemTotal: %lu", &mem_total);
        else if (line.find("MemAvailable:") == 0) sscanf(line.c_str(), "MemAvailable: %lu", &mem_avail);
    }
    total_kb = mem_total;
    used_kb = mem_total - mem_avail;
}

double read_disk_usage() {
    struct statvfs st;
    if (statvfs("/", &st) != 0) return 0.0;
    unsigned long long total = static_cast<unsigned long long>(st.f_blocks) * st.f_frsize;
    unsigned long long avail = static_cast<unsigned long long>(st.f_bavail) * st.f_frsize;
    if (total == 0) return 0.0;
    return 100.0 * (1.0 - static_cast<double>(avail) / total);
}

void read_disk_bytes(unsigned long long& total, unsigned long long& used) {
    struct statvfs st;
    total = used = 0;
    if (statvfs("/", &st) != 0) return;
    total = static_cast<unsigned long long>(st.f_blocks) * st.f_frsize;
    used = static_cast<unsigned long long>(st.f_blocks - st.f_bavail) * st.f_frsize;
}

long read_uptime() {
    std::ifstream f("/proc/uptime");
    if (!f) return 0;
    double up = 0;
    f >> up;
    return static_cast<long>(up);
}

void read_load_avg(double& l1, double& l5, double& l15) {
    std::ifstream f("/proc/loadavg");
    l1 = l5 = l15 = 0.0;
    if (!f) return;
    f >> l1 >> l5 >> l15;
}

std::string get_hostname() {
    char buf[256];
    if (gethostname(buf, sizeof(buf)) == 0)
        return std::string(buf);
    return "unknown";
}

std::string get_os_type() {
    struct utsname u;
    if (uname(&u) == 0) {
        return std::string(u.sysname) + " " + std::string(u.release);
    }
    return "Linux";
}

std::string generate_agent_id() {
    std::ifstream f("/etc/machine-id");
    std::string mid;
    if (f) std::getline(f, mid);
    if (mid.empty()) mid = "unknown";
    return mid + "-" + std::to_string(getpid()) + "-" + std::to_string(time(nullptr));
}

std::string get_ip_address() {
    FILE* fp = popen("hostname -I 2>/dev/null | awk '{print $1}'", "r");
    if (!fp) return "";
    char buf[64];
    std::string ip;
    if (fgets(buf, sizeof(buf), fp)) {
        ip = buf;
        while (!ip.empty() && (ip.back() == '\n' || ip.back() == '\r' || ip.back() == ' '))
            ip.pop_back();
    }
    pclose(fp);
    return ip;
}

// ---------------------------------------------------------------------------
// Agent registration
// ---------------------------------------------------------------------------
bool register_agent() {
    std::string ip = get_ip_address();
    std::ostringstream os;
    os << "{\"agent_id\":\"" << escape_json(AGENT_ID) << "\","
       << "\"hostname\":\"" << escape_json(AGENT_HOSTNAME) << "\","
       << "\"ip_address\":\"" << escape_json(ip) << "\","
       << "\"os_type\":\"" << escape_json(AGENT_OS) << "\"}";

    std::string url = std::string(API_URL) + "/api/v1/agents/register";
    Response resp = http_post(url, os.str());
    if (resp.http_code >= 200 && resp.http_code < 300) {
        log_msg(LOG_INFO, "Registered with backend successfully");
        return true;
    }
    log_msg(LOG_ERROR, "Registration failed: HTTP " + std::to_string(resp.http_code));
    return false;
}

// ---------------------------------------------------------------------------
// Telemetry
// ---------------------------------------------------------------------------
std::string build_telemetry_json() {
    double cpu = read_cpu_usage();
    double mem = read_memory_usage();
    unsigned long mem_total_kb = 0, mem_used_kb = 0;
    read_memory_bytes(mem_total_kb, mem_used_kb);
    double disk = read_disk_usage();
    unsigned long long disk_total = 0, disk_used = 0;
    read_disk_bytes(disk_total, disk_used);
    long uptime = read_uptime();
    double l1 = 0, l5 = 0, l15 = 0;
    read_load_avg(l1, l5, l15);

    std::ostringstream os;
    os << std::fixed;
    os << "{\"agent_id\":\"" << escape_json(AGENT_ID) << "\","
       << "\"hostname\":\"" << escape_json(AGENT_HOSTNAME) << "\","
       << "\"cpu_usage\":" << cpu << ","
       << "\"memory_usage\":" << mem << ","
       << "\"memory_total\":" << (mem_total_kb * 1024ULL) << ","
       << "\"memory_used\":" << (mem_used_kb * 1024ULL) << ","
       << "\"disk_usage\":" << disk << ","
       << "\"disk_total\":" << disk_total << ","
       << "\"disk_used\":" << disk_used << ","
       << "\"uptime_seconds\":" << uptime << ","
       << "\"load_avg_1\":" << l1 << ","
       << "\"load_avg_5\":" << l5 << ","
       << "\"load_avg_15\":" << l15 << "}";
    return os.str();
}

void send_telemetry() {
    std::string body = build_telemetry_json();
    std::string url = std::string(API_URL) + "/api/v1/telemetry";
    Response resp = http_post(url, body);
    if (resp.http_code >= 200 && resp.http_code < 300) {
        log_msg(LOG_INFO, "Telemetry sent successfully");
    } else {
        log_msg(LOG_WARN, "Telemetry send failed: HTTP " + std::to_string(resp.http_code));
    }
}

// ---------------------------------------------------------------------------
// Command polling & execution
// ---------------------------------------------------------------------------
struct CommandEntry {
    std::string id;
    std::string type;
};

std::vector<CommandEntry> parse_commands(const std::string& resp) {
    std::vector<CommandEntry> out;
    size_t pos = 0;
    while (true) {
        size_t id_pos = resp.find("\"id\"", pos);
        if (id_pos == std::string::npos) break;
        size_t colon = resp.find(':', id_pos);
        if (colon == std::string::npos) break;
        size_t q1 = resp.find('"', colon + 1);
        if (q1 == std::string::npos) break;
        size_t q2 = resp.find('"', q1 + 1);
        if (q2 == std::string::npos) break;
        std::string id = resp.substr(q1 + 1, q2 - q1 - 1);

        size_t type_pos = resp.find("\"command_type\"", q2);
        if (type_pos == std::string::npos || type_pos > id_pos + 500) {
            pos = id_pos + 1;
            continue;
        }
        size_t tc = resp.find(':', type_pos);
        size_t tq1 = resp.find('"', tc + 1);
        size_t tq2 = resp.find('"', tq1 + 1);
        if (tq2 == std::string::npos) break;
        std::string typ = resp.substr(tq1 + 1, tq2 - tq1 - 1);
        if (!id.empty() && !typ.empty()) {
            out.push_back({id, typ});
        }
        pos = tq2 + 1;
    }
    return out;
}

std::string execute_command(const std::string& cmd_type, bool& success) {
    success = true;
    if (cmd_type == "ping") {
        return "pong";
    }
    if (cmd_type == "collect_logs") {
        std::ifstream lf(LOG_FILE_PATH);
        if (!lf) return "No log file found";
        std::vector<std::string> lines;
        std::string line;
        while (std::getline(lf, line)) {
            lines.push_back(line);
            if (lines.size() > 50) lines.erase(lines.begin());
        }
        std::ostringstream os;
        for (const auto& l : lines) os << l << "\\n";
        return os.str();
    }
    if (cmd_type == "simulate_load") {
        log_msg(LOG_INFO, "Simulating CPU load for 3 seconds...");
        auto end = std::chrono::steady_clock::now() + std::chrono::seconds(3);
        volatile long x = 0;
        while (std::chrono::steady_clock::now() < end) { x++; }
        return "CPU load simulated for 3 seconds";
    }
    if (cmd_type == "restart_agent") {
        log_msg(LOG_INFO, "Restart command received (simulated)");
        return "Agent restart acknowledged (simulated)";
    }
    success = false;
    return "Unknown command type: " + cmd_type;
}

void report_command_result(const std::string& cmd_id, bool success,
                           const std::string& result) {
    std::ostringstream os;
    os << "{\"command_id\":\"" << escape_json(cmd_id) << "\","
       << "\"agent_id\":\"" << escape_json(AGENT_ID) << "\","
       << "\"status\":\"" << (success ? "success" : "failed") << "\","
       << "\"result\":\"" << escape_json(result) << "\"}";
    std::string url = std::string(API_URL) + "/api/v1/commands/result";
    Response resp = http_post(url, os.str());
    if (resp.http_code >= 200 && resp.http_code < 300) {
        log_msg(LOG_INFO, "Command result reported: " + cmd_id + " -> " +
                          (success ? "success" : "failed"));
    } else {
        log_msg(LOG_WARN, "Failed to report command result for " + cmd_id);
    }
}

void poll_and_execute_commands() {
    std::string url = std::string(API_URL) + "/api/v1/commands/pending?agent_id=" + AGENT_ID;
    Response resp = http_get(url);
    if (resp.http_code < 200 || resp.http_code >= 300) return;

    auto cmds = parse_commands(resp.data);
    for (const auto& cmd : cmds) {
        log_msg(LOG_INFO, "Executing command: " + cmd.type + " (id=" + cmd.id + ")");
        bool ok = false;
        std::string result = execute_command(cmd.type, ok);
        report_command_result(cmd.id, ok, result);
    }
}

}  // namespace

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char* argv[]) {
    // Open log file
    log_file.open(LOG_FILE_PATH, std::ios::app);
    if (!log_file.is_open()) {
        std::cerr << "Warning: cannot open log file " << LOG_FILE_PATH << std::endl;
    }

    // Configuration
    API_URL = getenv("TELEMETRY_API_URL");
    if (!API_URL || !*API_URL)
        API_URL = "http://localhost:8080";

    AGENT_ID = generate_agent_id();
    AGENT_HOSTNAME = get_hostname();
    AGENT_OS = get_os_type();

    curl_global_init(CURL_GLOBAL_DEFAULT);

    log_msg(LOG_INFO, "============================================");
    log_msg(LOG_INFO, "Telemetry Agent starting");
    log_msg(LOG_INFO, "  Agent ID : " + AGENT_ID);
    log_msg(LOG_INFO, "  Hostname : " + AGENT_HOSTNAME);
    log_msg(LOG_INFO, "  OS       : " + AGENT_OS);
    log_msg(LOG_INFO, "  API      : " + std::string(API_URL));
    log_msg(LOG_INFO, "============================================");

    // Step 1: Register with backend (retry until success)
    while (!register_agent()) {
        log_msg(LOG_WARN, "Registration failed, retrying in 5 seconds...");
        std::this_thread::sleep_for(std::chrono::seconds(5));
    }

    // Prime the CPU reading (first read is always 0)
    read_cpu_usage();
    std::this_thread::sleep_for(std::chrono::seconds(1));

    // Step 2: Main loop - telemetry every 5s, commands every 10s
    int tick = 0;
    while (true) {
        send_telemetry();

        if (tick % 2 == 0) {
            poll_and_execute_commands();
        }

        std::this_thread::sleep_for(std::chrono::seconds(TELEMETRY_INTERVAL_SEC));
        tick++;
    }

    curl_global_cleanup();
    log_file.close();
    return 0;
}
