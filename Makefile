# 
.DEFAULT_GOAL := help

.PHONY: help
help:
	@echo "This project is using mage (https://magefile.org) as a make replacement. "
	@echo "For help:                  go run mage.go -h"
	@echo "To build the project:      go run mage.go build:all"
	@echo "To run tests:              go run mage.go test:all"
	@echo "To get a list of targets:  go run mage.go -l"
	@echo
	@echo "The code for running the builds is located in ./build.go"