# Echo Logging Module

[![Go Reference](https://pkg.go.dev/github.com/altitude-analytics/echo?status.svg)](https://pkg.go.dev/github.com/altitude-analytics/echo)
A simple, configurable logging module for Go applications built upon the standard library's `log/slog` package. It supports structured logging to multiple destinations (console, file) with different formats (text, JSON).

## Features

* Built on standard `log/slog`.
* Configure log level (Debug, Info, Warn, Error).
* Output to Console (stdout) with Text or JSON format.
* Output to File with Text or JSON format.
* Optionally include source code location (file:line).
* Sets the default `slog` logger for easy integration.
* Basic error handling during initialization.

## Installation

```bash
go get [github.com/altitude-analytics/echo@latest](https://www.google.com/search?q=https://github.com/altitude-analytics/echo%40latest)```