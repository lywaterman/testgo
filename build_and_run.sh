#go build -ldflags '-linkmode "external" -extldflags "-static"' -o ./main
go build -o ./main
./main