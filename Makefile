run:
	go run *.go

windows:
	env GOOS=windows GOARCH=386 go build *.go

windows64:
	env GOOS=windows GOARCH=amd64 go build *.go
