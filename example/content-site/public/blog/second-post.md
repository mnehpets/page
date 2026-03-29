---
title: On Go Interfaces
linkTitle: Go Interfaces
date: 2026-03-15
tags: [go]
collection: blog
description: A look at how Go interfaces enable flexible, decoupled code without inheritance.
---

# On Go Interfaces

Go interfaces are satisfied implicitly — a type implements an interface simply by
having the right methods, with no declaration required.

```go
type Stringer interface {
    String() string
}

type Point struct{ X, Y int }

func (p Point) String() string {
    return fmt.Sprintf("(%d, %d)", p.X, p.Y)
}
```

This makes it easy to write code that accepts many types without coupling them
to a particular package.
