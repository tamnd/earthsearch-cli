---
title: "Resource URIs"
description: "Use earthsearch as a database/sql-style driver so a host program can address earthsearch as earthsearch:// URIs."
weight: 20
---

`earthsearch` is a command line, but the `earthsearch` Go package is also a
small driver that makes earthsearch addressable as a resource URI. A host
program registers it the way a program registers a database driver with
`database/sql`, then dereferences `earthsearch://` URIs without knowing
anything about how earthsearch is fetched.

The host that does this today is [ant](https://github.com/tamnd/ant), a single
binary that puts one URI namespace over a family of site tools. The examples
below use `ant`; any program that links the package gets the same behaviour.

## Mounting the driver

A host enables the driver with one blank import, exactly like `import _
"github.com/lib/pq"`:

```go
import _ "github.com/tamnd/earthsearch-cli/earthsearch"
```

The package's `init` registers a domain with the scheme `earthsearch` for the
host `earthsearch.com`. The standalone `earthsearch` binary does not change.

## Addressing records

A URI is `scheme://authority/id`. The scaffold ships one type:

| URI                              | What it is                              |
| -------------------------------- | --------------------------------------- |
| `earthsearch://page/<path>`    | a page, keyed by its path on earthsearch.com |

```bash
ant get earthsearch://page/<path>    # the page record
ant cat earthsearch://page/<path>    # just the body text
ant url earthsearch://page/<path>    # the live https URL
ant resolve https://earthsearch.com/<path> # a pasted link, back to its URI
```

As you add resolver operations in `earthsearch/domain.go`, each new `URIType`
becomes another addressable authority here, with no extra wiring. See
[add a command](/guides/adding-a-command/).

## Walking the graph

`ls` lists the members of a collection, and every member is itself an
addressable URI, so a host can follow the graph and write it to disk:

```bash
ant ls     earthsearch://page/<path>             # the pages this one links to
ant export earthsearch://page/<path> --follow 1 --to ./data
```

The example `links` op emits page stubs, so each listed member is a
`earthsearch://page/` URI in its own right. When you model edges between your
real records with `kit:"link"` tags, `ant export --follow` and `ant graph` walk
those edges too, across tools when a link points at another site's scheme.

## Why this is the same code

The driver and the binary share one definition per operation. A resolver op
answers both `earthsearch page` on the command line and `ant get
earthsearch://page/...` through a host, from the same handler and the same
client. There is no second implementation to keep in step.
