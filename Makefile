build:
	go build -o runx
cp:
	cp runx ~/.local/bin/

install: build cp
