cmake_minimum_required(VERSION 3.16)
project(TangoFcitx5Bridge LANGUAGES CXX)
find_package(Fcitx5Core REQUIRED)
find_package(Threads REQUIRED)

set(TANGO_FCITX_ADDON "" CACHE STRING "Fcitx addon name")
set(TANGO_FCITX_CONFIG "" CACHE FILEPATH "Installed runtime configuration")
if(NOT TANGO_FCITX_ADDON OR NOT TANGO_FCITX_CONFIG)
    message(FATAL_ERROR "Specify TANGO_FCITX_ADDON and TANGO_FCITX_CONFIG")
endif()

add_library(tango_fcitx MODULE tangobridge.cpp pipe.cpp)
target_compile_features(tango_fcitx PRIVATE cxx_std_17)
target_compile_definitions(tango_fcitx PRIVATE
    "TANGO_FCITX_ADDON=\"${TANGO_FCITX_ADDON}\""
    "TANGO_FCITX_CONFIG=\"${TANGO_FCITX_CONFIG}\"")
target_link_libraries(tango_fcitx PRIVATE Fcitx5::Core Threads::Threads)
set_target_properties(tango_fcitx PROPERTIES PREFIX "" OUTPUT_NAME "${TANGO_FCITX_ADDON}")
