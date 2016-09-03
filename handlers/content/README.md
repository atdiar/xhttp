#content

[![GoDoc](https://godoc.org/github.com/atdiar/xhttp/handlers/content?status.svg)](https://godoc.org/github.com/atdiar/xhttp/handlers/content)

This package defines a content serving request handler, allowing to serve http
range requests.

##Dependencies

* [Package xhttp]
* [Package execution]

These are the only two external dependencies required as they are necessary
to take into account the execution context of a request-handling goroutine.

##License

MIT

[Package xhttp]:http://github.com/atdiar/xhttp
[Package execution]:http://github.com/atdiar/goroutine/execution
