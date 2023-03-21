package main

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

type InspectAction struct {
	Url     string
	Handler func(body []byte)
}

type InspectActionPool struct {
	sync.Mutex
	Actions map[int64]*InspectAction
}

func (p *InspectActionPool) Add(url string, handler func([]byte)) func() {
	p.Lock()
	defer p.Unlock()

	idnya := time.Now().Unix()
	cancel := func() {
		p.Lock()
		defer p.Unlock()

	}

	action := InspectAction{
		Url: url,
		Handler: func(body []byte) {
			defer delete(p.Actions, idnya)
			handler(body)
		},
	}

	p.Actions[idnya] = &action

	return cancel
}

type InspectDriver struct {
	ListActions *InspectActionPool
	Ctx         context.Context
	Cancel      func()
}

func NewInspectDriver() (*InspectDriver, error) {

	opts := []func(*chromedp.ExecAllocator){
		// chromedp.Flag("headless", headless),

		// chromedp.UserDataDir(pathdata),
		// chromedp.Flag("profile-directory", "Default"),
	}
	// creating selenium context
	ctx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelCtx := chromedp.NewContext(ctx)

	// creating action pool context
	actions := InspectActionPool{
		Actions: map[int64]*InspectAction{},
	}

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *network.EventResponseReceived:

			go func() {
				actions.Lock()
				defer actions.Unlock()

				// getting response body
				c := chromedp.FromContext(ctx)
				body, _ := network.GetResponseBody(ev.RequestID).Do(cdp.WithExecutor(ctx, c.Target))
				url := ev.Response.URL

				for _, action := range actions.Actions {
					if strings.Contains(url, action.Url) {
						if string(body) == "" {
							continue
						}
						action.Handler(body)
					}

				}

				fetch.ContinueRequest(fetch.RequestID(ev.RequestID))

			}()
		case *fetch.EventRequestPaused:
			go func() {
				c := chromedp.FromContext(ctx)
				e := cdp.WithExecutor(ctx, c.Target)
				req := fetch.ContinueRequest(ev.RequestID)
				req.Do(e)
			}()
		}
	})

	driver := InspectDriver{
		ListActions: &actions,
		Ctx:         ctx,
		Cancel: func() {
			defer cancelAlloc()
			defer cancelCtx()

		},
	}

	return &driver, nil

}

func main() {
	driver, _ := NewInspectDriver()
	defer driver.Cancel()

	driver.ListActions.Add("/janus/v1/app-auth/login", func(b []byte) {
		log.Println(string(b))
	})

	err := chromedp.Run(driver.Ctx,
		fetch.Enable(),
		chromedp.Navigate("https://shopee.co.id"),
	)

	time.Sleep(time.Second * 5)

	chromedp.Run(driver.Ctx,
		fetch.Enable(),
		chromedp.Navigate("https://shopee.co.id"),
	)

	if err != nil {
		log.Fatal(err)
	}
}
