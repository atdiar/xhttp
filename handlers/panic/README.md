# panic
[![GoDoc](https://godoc.org/github.com/atdiar/xhttp/handlers/panic?status.svg)](https://godoc.org/github.com/atdiar/xhttp/handlers/panic)

This package implements a request handler for incoming http requests.
It allows to specify what should be done by the program in case a panic
occurs during the request handling.

## Usage recommendation

It is recommended for this request handler to be registered early.
Most chains of request handlers should end up calling it in order to handle any
potential panics they could have triggered.

We remind the programmer that request handlers are called in the reverse order
of their registration/linking, i.e. last gets executed first.

## How to use it?

The handler is implemented as follows:

``` go
type Handler struct {
	Handle func(ctx execution.Context, w http.ResponseWriter, r *http.Request)
	Log    func(data ...interface{})
	next   xhttp.Handler
}

```
The `Handle` field shall be provided by the programmer. It implements the panic
handling logic.

The `Log` field allows to provide a logging function, should the programmer feel
the need to provide a way to record abnormal behaviour.

## Dependencies
This package depends on:
* [Execution Context package](https://github.com/atdiar/goroutine/execution)
* [xhttp package](https://github.com/atdiar/xhttp)

## License
MIT
