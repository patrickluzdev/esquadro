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
