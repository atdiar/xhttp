# hsts

[![GoDoc](https://godoc.org/github.com/atdiar/xhttp/handlers/hsts?status.svg)](https://godoc.org/github.com/atdiar/xhttp/handlers/hsts)

This package defines a hsts enabling request handler for strict transport security.

## Dependencies

* [Package xhttp]
* [Package execution]

These are the only two external dependencies required as they are necessary
to take into account the execution context of a request-handling goroutine.

## License

BSD 3-clause

[Package xhttp]:http://github.com/atdiar/xhttp
[Package execution]:http://github.com/atdiar/goroutine/execution
