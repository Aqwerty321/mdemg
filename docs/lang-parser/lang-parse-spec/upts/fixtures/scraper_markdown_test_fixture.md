# net/http — Go Standard Library

Package net/http provides HTTP client and server implementations.

## Overview

The http package provides an HTTP client and server. Get, Head, Post, and PostForm
make HTTP requests. The Client type handles redirects, cookies, timeouts, and
connection pooling automatically.

For control over HTTP client headers, redirect policy, and other settings, create
a Client and use its Do method with a custom Request.

## Types

### Handler

A Handler responds to an HTTP request. ServeHTTP should write reply headers and data
to the ResponseWriter and then return.

```go
type Handler interface {
    ServeHTTP(ResponseWriter, *Request)
}
```

### Request

A Request represents an HTTP request received by a server or to be sent by a client.

```go
type Request struct {
    Method string
    URL    *url.URL
    Header Header
    Body   io.ReadCloser
}
```

### ResponseWriter

A ResponseWriter interface is used by an HTTP handler to construct an HTTP response.

## Functions

### ListenAndServe

ListenAndServe listens on the TCP network address addr and then calls Serve to
handle requests on incoming connections.

```go
func ListenAndServe(addr string, handler Handler) error
```

### HandleFunc

HandleFunc registers the handler function for the given pattern in the DefaultServeMux.

```go
func HandleFunc(pattern string, handler func(ResponseWriter, *Request))
```

## Examples

A simple HTTP server:

```go
http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "Hello, World!")
})
http.ListenAndServe(":8080", nil)
```

Using the client:

```python
import requests
response = requests.get("http://localhost:8080/hello")
print(response.text)
```

Shell testing:

```bash
curl http://localhost:8080/hello
```

## Related Packages

[net/url](https://pkg.go.dev/net/url)
[io](https://pkg.go.dev/io)
[context](https://pkg.go.dev/context)

## See Also

[HTTP specification](https://www.rfc-editor.org/rfc/rfc9110)
[Go documentation](https://pkg.go.dev/net/http)
