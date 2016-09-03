#stripprefix

[![GoDoc](https://godoc.org/github.com/atdiar/xhttp/handlers/stripprefix?status.svg)](https://godoc.org/github.com/atdiar/xhttp/handlers/stripprefix)

This package defines a request handler that simply attempts to mutatet the request object
by stripping a given prefix from it.

##Dependencies

* [Package xhttp]
* [Package execution]

These are the only two external dependencies required as they are necessary
to take into account the execution context of a request-handling goroutine.

##License

MIT

[Package xhttp]:http://github.com/atdiar/xhttp
[Package execution]:http://github.com/atdiar/goroutine/execution
