package main

import (
	"context"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func CreateNeoSession() (neo4j.SessionWithContext, func()) {
	uri := "neo4j+s://c1e7f1ad.databases.neo4j.io"
	auth := neo4j.BasicAuth("neo4j", "bECIGSEz_Xig0mBmD_twaCFEByseyr54HvqBzQIb_wk", "")

	driver, err := neo4j.NewDriverWithContext(uri, auth)
	if err != nil {
		panic(err)
	}
	ctx := context.Background()

	session := driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})

	return session, func() {
		session.Close(ctx)
		driver.Close(ctx)
	}
}
