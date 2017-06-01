#redirect

[![GoDoc](https://godoc.org/github.com/atdiar/xhttp/handlers/redirect?status.svg)](https://godoc.org/github.com/atdiar/xhttp/handlers/redirect)

This package defines a request handler in charge of replying to an incoming
request by sending a redirection response.

##Dependencies

* [Package xhttp]
* [Package execution]

These are the only two external dependencies required as they are necessary
to take into account the execution context of a request-handling goroutine.

##License

BSD 3-clause

[Package xhttp]:http://github.com/atdiar/xhttp
[Package execution]:http://github.com/atdiar/goroutine/execution
