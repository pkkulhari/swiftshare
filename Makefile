.PHONY: run build-linux build-windows

run:
	go run .

build-linux:
	go build -ldflags "-s -w" -o Swiftshare .

build-windows:
	go build -ldflags "-s -w" -o Swiftshare.exe .