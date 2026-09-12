.PHONY: page check-page format check-format contract check-contract

page:
	go run . -export > docs/index.html

check-page:
	python3 scripts/check-page.py

format:
	gofmt -w main.go main_test.go cmd test-cases

check-format:
	@test -z "$$(gofmt -l main.go main_test.go cmd test-cases)" || { gofmt -l main.go main_test.go cmd test-cases; exit 1; }

contract:
	python3 scripts/contract.py

check-contract:
	python3 scripts/contract.py --check
