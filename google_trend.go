package main

import (
	"net/url"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/chromedp"
	"github.com/gorilla/schema"
)

type GoogleTrendQuery struct {
	Q string `schema:"q"`
}

var encoder = schema.NewEncoder()

func (d *Dataset) GetGoogleTrend(query *GoogleTrendQuery) {
	// setup url
	u, _ := url.Parse("https://trends.google.com/trends/explore")
	q := u.Query()
	encoder.Encode(&query, q)
	u.RawQuery = q.Encode()

	chromedp.Run(
		d.ChromeCtx,
		fetch.Enable(),
		chromedp.Navigate(u.String()),
	)
}
