install-cli:
	go install ./cmd/barry
	barry help

test:
	go test -cover ./... 

coverage:
	go test -coverprofile=coverage.out ./...  
	go tool cover -html=coverage.out
