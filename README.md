# GCR - Global Context Repository

[![Build Status][BS img]][Build Status]
[![Coverage Status][CS img]][Coverage Status]

## Author's POV

Sharing variables between request is not straight forward when building [go]
applications.

When you have a domain driven approach to your web application design where
you package everything into it's own domain it  becomes tricky when passing
in values/references around during the lifetime of a web request or even
during your unit tests.

Most web frameworks in [go] have a built in contexts that handles this quite
well actually, eg. [gorilla context], [gin].

But often times you just want to use the standard [net package] in [go] which
is already super awesome. So if that's the case you want a framework agnostic
context management solution.

This package tries to solve that by building around the [go context] package.

## Usage

```go
package main

import (
    "fmt"
    "net/http"
    "time"

    "golang.org/x/net/context"

    "github.com/zenazn/goji/web"
    "github.com/tylerb/graceful"
)

func main() {
    // using goji web
    mux := web.New()
    // Start global context repository
    gcr.Start(nil) // you can pass in options if you wish

    mux.Handle("/search/manbearpig", controller.Foo())
    // graceful start/stop server
    srv := &graceful.Server{
        Server: &http.Server{
            Addr: "locahost:3000",
            // This is important, we need to clear the context per each
            // request so this must be set on the top level handler
            Handler: gcr.NewClearContextHandler(mux),
        },
    }
    srv.ListenAndServe()
}
```

[go]: https://golang.org/
[gorilla context]: http://www.gorillatoolkit.org/pkg/context
[gin]: https://gin-gonic.github.io/gin/
[go context]: https://godoc.org/golang.org/x/net/context
[net package]: https://golang.org/pkg/net/http

[Build Status]: https://travis-ci.org/magicalbanana/gcr
[BS img]: https://travis-ci.org/magicalbanana/gcr.svg

[Coverage Status]: https://coveralls.io/r/magicalbanana/gcr
[CS img]: https://coveralls.io/repos/magicalbanana/gcr/badge.svg?branch=master&service=github
