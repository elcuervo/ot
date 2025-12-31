default:
  @just --list

test:
  go test -v ./...

test-coverage:
  go test -v -coverprofile=coverage.out ./...
  go tool cover -html=coverage.out -o coverage.html

test-watch:
  find . -name '*.go' | entr -c go test -v ./...

build:
  go build -ldflags "-X main.buildSHA=$(git rev-parse --short HEAD)" -o ot

install:
  go install .

nix-install:
  nix profile remove ot 2>/dev/null || true
  nix profile install .#ot --refresh --no-substitute

tag version:
  git tag "v{{version}}"

release version:
  git tag "v{{version}}"
  git push origin "v{{version}}"

fmt:
  go fmt ./...

vet:
  go vet ./...

lint: fmt vet

demo: build
  ./ot --vault ./examples/vault ./examples/query.md

gif:
  vhs -p demo.tape

clean:
  rm -f ot coverage.out coverage.html

run vault query:
  go run . --vault {{vault}} {{query}}

list vault query:
  go run . --vault {{vault}} --list {{query}}
