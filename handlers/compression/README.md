#compression

[![GoDoc](https://godoc.org/github.com/atdiar/xhttp/handlers/compression?status.svg)](https://godoc.org/github.com/atdiar/xhttp/handlers/compression)

This package defines a request handler in charge of gzip compressing the
 response to an incoming http request.

It conforms to the signature of linkable handlers as defined by the xhttp
package, by implementing the xhttp.HandlerLinker interface.

It is possible to disable gzip compression per request in order to avoid some
CSRF vulnerabilities.

##How to use it?

It is typically used early in the request handling process as a catch-all-routes
handler.
Below, is a contrieved example (the imports are not showed)

``` go

mux := xhttp.NewServeMux()

compressor := NewHandler().Skip("POST")

mux.USE(compressor)

```
##Dependencies

[xhttp package]:http://github.com/atdiar/xhttp
[execution package]:http://github.com/atdiar/goroutine/execution

These are the only two external dependencies required as they are necessary
to take into account the execution context of a request handling goroutine.
