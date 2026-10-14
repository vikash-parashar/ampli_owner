TEST_DATABASE_URL ?= postgres://ampli:ampli@localhost:5432/ampli_test?sslmode=disable

.PHONY: run test lambda
run:
	docker compose up --build

test:
	TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test ./... -count=1

# bootstrap binary for the provided.al2023 Lambda runtime (arm64 is cheaper)
lambda:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -trimpath -ldflags="-s -w" -o build/bootstrap ./cmd/server
	cd build && zip -q ampli-owner-lambda.zip bootstrap
