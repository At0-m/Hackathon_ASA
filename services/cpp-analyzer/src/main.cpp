#include "http_server.h"

#include <cstdlib>
#include <iostream>

int main() {
  int port = 9090;
  if (const char* env = std::getenv("ANALYZER_PORT")) {
    port = std::atoi(env);
  }
  try {
    aca::RunServer(port);
  } catch (const std::exception& e) {
    std::cerr << "analyzer error: " << e.what() << std::endl;
    return 1;
  }
  return 0;
}
