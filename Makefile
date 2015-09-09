run:
	go run *.go

windows:
	env GOOS=windows GOARCH=386 go build *.go
