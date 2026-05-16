cmake_minimum_required(VERSION 3.20)
project(aca_cpp_analyzer CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
add_compile_options(-Wall -Wextra -Wpedantic)

add_executable(ai_analyzer
  src/main.cpp
  src/analyzer.cpp
  src/http_server.cpp
)

target_include_directories(ai_analyzer PRIVATE src)
target_link_libraries(ai_analyzer PRIVATE pthread)
