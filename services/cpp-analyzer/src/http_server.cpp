#include "http_server.h"

#include "analyzer.h"

#include <arpa/inet.h>
#include <netinet/in.h>
#include <sys/socket.h>
#include <unistd.h>

#include <cstring>
#include <iostream>
#include <sstream>
#include <string>
#include <thread>

namespace aca {
namespace {

std::string ReadRequest(int fd) {
  std::string data;
  char buffer[4096];
  while (true) {
    ssize_t n = recv(fd, buffer, sizeof(buffer), 0);
    if (n <= 0) break;
    data.append(buffer, buffer + n);
    auto pos = data.find("\r\n\r\n");
    if (pos != std::string::npos) {
      size_t content_length = 0;
      auto cl = data.find("Content-Length:");
      if (cl != std::string::npos && cl < pos) {
        cl += std::strlen("Content-Length:");
        while (cl < data.size() && data[cl] == ' ') ++cl;
        content_length = static_cast<size_t>(std::stoul(data.substr(cl)));
      }
      if (data.size() >= pos + 4 + content_length) break;
    }
  }
  return data;
}

std::string BodyOf(const std::string& request) {
  auto pos = request.find("\r\n\r\n");
  if (pos == std::string::npos) return "";
  return request.substr(pos + 4);
}

std::string PathOf(const std::string& request) {
  auto first = request.find(' ');
  if (first == std::string::npos) return "/";
  auto second = request.find(' ', first + 1);
  if (second == std::string::npos) return "/";
  return request.substr(first + 1, second - first - 1);
}

std::string MethodOf(const std::string& request) {
  auto first = request.find(' ');
  if (first == std::string::npos) return "";
  return request.substr(0, first);
}

void Send(int fd, int status, const std::string& body) {
  std::string reason = status == 200 ? "OK" : status == 404 ? "Not Found" : "Bad Request";
  std::ostringstream os;
  os << "HTTP/1.1 " << status << " " << reason << "\r\n";
  os << "Content-Type: application/json; charset=utf-8\r\n";
  os << "Access-Control-Allow-Origin: *\r\n";
  os << "Access-Control-Allow-Methods: GET, POST, OPTIONS\r\n";
  os << "Access-Control-Allow-Headers: Content-Type, Authorization\r\n";
  os << "Content-Length: " << body.size() << "\r\n";
  os << "Connection: close\r\n\r\n";
  os << body;
  const std::string response = os.str();
  send(fd, response.data(), response.size(), 0);
}

void HandleClient(int fd) {
  const std::string request = ReadRequest(fd);
  const std::string method = MethodOf(request);
  const std::string path = PathOf(request);
  if (method == "OPTIONS") {
    Send(fd, 200, "{\"status\":\"ok\"}");
  } else if (method == "GET" && path == "/health") {
    Send(fd, 200, "{\"status\":\"ok\"}");
  } else if (method == "POST" && path == "/analyze") {
    try {
      Send(fd, 200, AnalyzeSessionJson(BodyOf(request)));
    } catch (const std::exception& e) {
      Send(fd, 400, std::string("{\"error\":\"") + e.what() + "\"}");
    }
  } else {
    Send(fd, 404, "{\"error\":\"not found\"}");
  }
  close(fd);
}

}  // namespace

void RunServer(int port) {
  int server_fd = socket(AF_INET, SOCK_STREAM, 0);
  if (server_fd < 0) {
    throw std::runtime_error("socket failed");
  }
  int opt = 1;
  setsockopt(server_fd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));

  sockaddr_in address{};
  address.sin_family = AF_INET;
  address.sin_addr.s_addr = INADDR_ANY;
  address.sin_port = htons(static_cast<uint16_t>(port));
  if (bind(server_fd, reinterpret_cast<sockaddr*>(&address), sizeof(address)) < 0) {
    throw std::runtime_error("bind failed");
  }
  if (listen(server_fd, 64) < 0) {
    throw std::runtime_error("listen failed");
  }
  std::cout << "ACA C++ analyzer listening on :" << port << std::endl;
  while (true) {
    int fd = accept(server_fd, nullptr, nullptr);
    if (fd < 0) continue;
    std::thread(HandleClient, fd).detach();
  }
}

}  // namespace aca
