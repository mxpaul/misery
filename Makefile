all:
	@echo Need target

tidyvendor:
	go mod tidy
	GOWORK=off go mod vendor
