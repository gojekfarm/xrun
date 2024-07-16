# xrun 

[![test][github-workflow-badge]][github-workflow]
[![codecov][coverage-badge]][codecov]
[![PkgGoDev][pkg-go-dev-xrun-badge]][pkg-go-dev-xrun]
[![Go Report Card][go-report-card-badge]][go-report-card]

> Utilities around running multiple components
> which are long-running components, example: 
> an HTTP server or a background worker

## Install

```
$ go get github.com/gojekfarm/xrun
```

## Usage

> Minimum Required Go Version: 1.20.x

- [API reference][api-docs]
- [Blog post explaining motivation behind xrun][blog-link]
- [Reddit post][reddit-link]

###### Credits

Manager source modified
from [sigs.k8s.io/controller-runtime](https://github.com/kubernetes-sigs/controller-runtime/tree/a1e2ea2/pkg/manager)

[github-workflow-badge]:
https://github.com/gojekfarm/xrun/workflows/test/badge.svg
[github-workflow]:
https://github.com/gojekfarm/xrun/actions?query=workflow%3Atest
[coverage-badge]: https://codecov.io/gh/gojekfarm/xrun/branch/main/graph/badge.svg?token=QPLV2ZDE84
[codecov]: https://codecov.io/gh/gojekfarm/xrun
[pkg-go-dev-xrun-badge]: https://pkg.go.dev/badge/github.com/gojekfarm/xrun
[pkg-go-dev-xrun]: https://pkg.go.dev/mod/github.com/gojekfarm/xrun?tab=packages
[go-report-card-badge]: https://goreportcard.com/badge/github.com/gojekfarm/xrun
[go-report-card]: https://goreportcard.com/report/github.com/gojekfarm/xrun
[api-docs]: https://pkg.go.dev/github.com/gojekfarm/xrun
[blog-link]: https://ajatprabha.in/2023/05/24/intro-xrun-package-managing-component-lifecycle-go
[reddit-link]: https://www.reddit.com/r/golang/comments/13r91gt/introducing_xrun_a_flexible_package_for_managing

