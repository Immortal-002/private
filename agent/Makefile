CXX = g++
CXXFLAGS = -std=c++17 -O2 -Wall
LDFLAGS = -lcurl -lpthread

SRC = src/main.cpp
OUT = telemetry_agent

.PHONY: all clean

all: $(OUT)

$(OUT): $(SRC)
	$(CXX) $(CXXFLAGS) -o $(OUT) $(SRC) $(LDFLAGS)

clean:
	rm -f $(OUT)
