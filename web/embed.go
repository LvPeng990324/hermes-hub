package web

import "embed"

// DistFS 内嵌前端构建产物（web/dist），实现单二进制交付
//
//go:embed all:dist
var DistFS embed.FS
