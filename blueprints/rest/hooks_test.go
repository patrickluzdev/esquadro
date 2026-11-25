package rest

import (
	"strings"
	"testing"
)

func TestServerDocument(t *testing.T) {
	published := `openapi: 3.0.3
servers:
  - url: /
paths:
  /api/v1/products:
    get:
      operationId: listProducts
  /identity/customer/sign-in:
    post:
      operationId: signIn
components:
  schemas:
    NewProduct:
      properties:
        name:
          type: string
          minLength: 2` + ruleMark + `
        placed: {type: string, format: date-time, nullable: true}
        weight: {type: string, format: decimal, nullable: true}
    IdentitySession:
      type: object
    Problem:
      type: object
`
	want := `openapi: 3.0.3
servers:
  - url: /api/v1
paths:
  /products:
    get:
      operationId: listProducts
components:
  schemas:
    NewProduct:
      properties:
        name:
          type: string
        placed: {type: string, format: date-time, x-ogen-time-format: "2006-01-02T15:04:05.999999Z07:00", nullable: true}
        weight: {type: string, nullable: true}
    Problem:
      type: object
`
	if got := string(ServerDocument([]byte(published))); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(want, "Identity") {
		t.Fatal("the test is wrong")
	}
}
