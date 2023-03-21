package main

import (
	"context"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type Dataset struct {
	NeoCtx    neo4j.SessionWithContext
	ChromeCtx context.Context
}

type GoogleKeyword struct {
	Index int64
	Key   string
}

func (d *Dataset) AddGoogleKeyword(keyword string) error {
	result := GoogleKeyword{
		Key: keyword,
	}

	path := `//*[@name="q"]`
	pathStat := `//*/div[@id="result-stats"]`

	chromedp.Run(
		d.ChromeCtx,
		chromedp.Navigate("https://google.com"),
		chromedp.WaitVisible(path, chromedp.BySearch),
		chromedp.SendKeys(path, keyword, chromedp.BySearch),
		chromedp.ActionFunc(func(ctx context.Context) error {
			relateds := ExtractRelated(ctx, keyword)

			if len(relateds) == 0 {
				return nil
			}

			txctx := context.Background()
			d.NeoCtx.ExecuteWrite(txctx, func(tx neo4j.ManagedTransaction) (any, error) {
				query := `
					MERGE (k:Keyword { key: $OriginKey })
					MERGE (k2:Keyword { key: $Key })
					MERGE (k)<-[:RELATED{ search_engine: "google" }]-(k2)
					RETURN k, k2
				`
				for _, related := range relateds {
					result, _ := tx.Run(txctx, query, map[string]any{
						"Key":       related,
						"OriginKey": keyword,
					})

					result.Collect(txctx)
				}

				return nil, nil

			})
			return nil
		}),
		chromedp.SendKeys(path, kb.Enter, chromedp.BySearch),
		chromedp.WaitVisible(pathStat, chromedp.BySearch),
		chromedp.ActionFunc(func(ctx context.Context) error {

			var stat string
			err := chromedp.Text(pathStat, &stat, chromedp.BySearch).Do(ctx)
			if err != nil {
				return err
			}

			r := regexp.MustCompile(`[0-9\.]+`)
			data := r.Find([]byte(stat))

			incount := strings.ReplaceAll(string(data), ".", "")
			index, err := strconv.Atoi(incount)
			if err != nil {
				return err
			}

			result.Index = int64(index)

			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {

			txctx := context.Background()
			d.NeoCtx.ExecuteWrite(txctx, func(tx neo4j.ManagedTransaction) (any, error) {
				query := `
					MERGE (k:Keyword { key: $Key })
					MERGE (s:Search { search_engine: "google", key: $Key})
					MERGE (k)-[:SEARCH{index_search: $Index}]->(s)
					RETURN k, s
				`

				result, _ := tx.Run(txctx, query, map[string]any{
					"Key":   result.Key,
					"Index": result.Index,
				})

				return result.Collect(txctx)

			})
			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			d.InspectShopping(ctx, keyword)

			return nil
		}),
	)

	return nil
}

func ExtractRelated(ctx context.Context, keyword string) []string {
	time.Sleep(time.Second * 3)

	relatedPath := `//*/div[@role="presentation"]/span`
	var nodes []*cdp.Node
	chromedp.Nodes(relatedPath, &nodes, chromedp.BySearch).Do(ctx)
	result := []string{}

	for _, node := range nodes {
		var key string
		chromedp.Text([]cdp.NodeID{node.NodeID}, &key, chromedp.ByNodeID).Do(ctx)

		if key == "" {
			continue
		}
		if key == keyword {
			continue
		}

		result = append(result, key)
	}
	log.Println(result)
	return result

}

type SourceType string

type SourceLink struct {
	Source string
	Link   string
}

func NewSourceLink(link string) *SourceLink {
	source := SourceLink{}

	u, _ := url.Parse(link)

	// checking dari google
	urlq := u.Query().Get("url")
	if urlq != "" {
		source.Link = urlq
		uq, _ := url.Parse(urlq)
		source.Source = uq.Hostname()

		return &source

	}

	log.Println("link not categorized", link)
	return &source
}

type GoogleShopProduct struct {
	*SourceLink
	Title string
	Price int64
}

type Price struct {
	Currency string
	Value    int64
}

func formatPrice(data string) *Price {
	result := Price{}

	belakang := regexp.MustCompile(`[\,\.]?[0]{2}$`)
	regp := regexp.MustCompile(`[0-9\,\.]+`)
	curreg := regexp.MustCompile(`[a-zA-Z]+`)
	replapuc := regexp.MustCompile(`[\,\.]`)

	repdata := belakang.ReplaceAllString(data, "")
	currency := curreg.FindString(repdata)
	result.Currency = currency

	pricestr := regp.FindString(repdata)
	pricestr = replapuc.ReplaceAllString(pricestr, "")
	price, _ := strconv.ParseInt(pricestr, 10, 64)
	result.Value = price

	return &result

}

func ExtractGoogleShopProduct(ctx context.Context, node *cdp.Node) *GoogleShopProduct {
	data := GoogleShopProduct{}

	titleNodeId, _ := dom.QuerySelector(node.NodeID, "h3").Do(ctx)
	chromedp.Text([]cdp.NodeID{titleNodeId}, &data.Title, chromedp.ByNodeID).Do(ctx)

	priceId, _ := dom.QuerySelector(node.NodeID, `span span[aria-hidden="true"] span`).Do(ctx)
	var strPrice string
	chromedp.Text([]cdp.NodeID{priceId}, &strPrice, chromedp.ByNodeID).Do(ctx)
	price := formatPrice(strPrice)
	data.Price = price.Value

	linkId, _ := dom.QuerySelector(node.NodeID, `a[data-what="1"]`).Do(ctx)
	var linkdata string
	var ok bool
	chromedp.AttributeValue([]cdp.NodeID{linkId}, "href", &linkdata, &ok, chromedp.ByNodeID).Do(ctx)

	source := NewSourceLink(linkdata)
	data.SourceLink = source

	return &data
}

func (d *Dataset) InspectShopping(ctx context.Context, keyword string) error {
	shopbutton := `//*/div[@class="hdtb-mitem"]/a[text()[contains(., "Shopping")]]`
	productPath := `//*/div[@data-docid]`

	products := []*GoogleShopProduct{}

	// getting product
	chromedp.Run(
		ctx,
		chromedp.WaitVisible(shopbutton, chromedp.BySearch),
		chromedp.Click(shopbutton, chromedp.BySearch),

		chromedp.ActionFunc(func(ctx context.Context) error {
			time.Sleep(time.Second * 3)
			runtime.Evaluate(`window.scrollTo(0,document.body.scrollHeight);`).Do(ctx)

			var nodes []*cdp.Node
			chromedp.Nodes(productPath, &nodes, chromedp.BySearch).Do(ctx)

			for _, node := range nodes {
				product := ExtractGoogleShopProduct(ctx, node)
				products = append(products, product)
			}

			return nil
		}),
	)

	log.Println(keyword, "getting", len(products))

	// saving product
	txctx := context.Background()
	d.NeoCtx.ExecuteWrite(txctx, func(tx neo4j.ManagedTransaction) (any, error) {
		query := `

			MATCH (k:Keyword { key: $Key })
			MERGE (p: Product {
				Title: $Title,
				Price: $Price,
				Source: $Source,
				Link: $Link
			})

			MERGE (p)<-[:FOUND{ search_engine: "google", page: 1}]-(k)

			RETURN p
		`
		for _, product := range products {
			result, _ := tx.Run(txctx, query, map[string]any{
				"Key":    keyword,
				"Title":  product.Title,
				"Price":  product.Price,
				"Source": product.Source,
				"Link":   product.Link,
			})

			result.Collect(txctx)
		}

		return nil, nil

	})

	return nil
}

func NewDataset() *Dataset {
	driver, _ := NewInspectDriver()
	session, _ := CreateNeoSession()

	dataset := Dataset{
		NeoCtx:    session,
		ChromeCtx: driver.Ctx,
	}

	return &dataset
}
