package main

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func Teast() {
	// Aura requires you to use "neo4j+s" protocol
	// (You need to replace your connection details, username and password)
	uri := "neo4j+s://c1e7f1ad.databases.neo4j.io"
	auth := neo4j.BasicAuth("neo4j", "bECIGSEz_Xig0mBmD_twaCFEByseyr54HvqBzQIb_wk", "")
	// You typically have one driver instance for the entire application. The
	// driver maintains a pool of database connections to be used by the sessions.
	// The driver is thread safe.
	driver, err := neo4j.NewDriverWithContext(uri, auth)
	if err != nil {
		panic(err)
	}
	// You can specify custom contexts to control the execution of the driver operations
	// This one never cancels, never times out and is used in subsequent calls
	// You can instead specify different contexts for different operations
	// Read more about contexts in https://pkg.go.dev/context
	ctx := context.Background()
	// Don't forget to close the driver connection when you are finished with it
	defer driver.Close(ctx)
	// Create a session to run transactions in. Sessions are lightweight to
	// create and use. Sessions are NOT thread safe.
	session := driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer session.Close(ctx)
	// WriteTransaction retries the operation in case of transient errors by
	// invoking the anonymous function multiple times until it succeeds.
	records, err := session.ExecuteWrite(ctx,
		func(tx neo4j.ManagedTransaction) (any, error) {
			// To learn more about the Cypher syntax, see https://neo4j.com/docs/cypher-manual/current/
			// The Reference Card is also a good resource for keywords https://neo4j.com/docs/cypher-refcard/current/
			createRelationshipBetweenPeopleQuery := `
				MERGE (p1:Person { name: $person1_name })
				MERGE (p2:Person { name: $person2_name })
				MERGE (p1)-[:KNOWS]-(p2)
				RETURN p1, p2`
			result, err := tx.Run(ctx, createRelationshipBetweenPeopleQuery, map[string]any{
				"person1_name": "Alice",
				"person2_name": "David",
			})
			if err != nil {
				// Return the error received from driver here to indicate rollback,
				// the error is analyzed by the driver to determine if it should try again.
				return nil, err
			}
			// Collects all records and commits the transaction (as long as
			// Collect doesn't return an error).
			// Beware that Collect will buffer the records in memory.
			return result.Collect(ctx)
		})
	if err != nil {
		panic(err)
	}
	for _, record := range records.([]*neo4j.Record) {
		firstPerson := record.Values[0].(neo4j.Node)
		fmt.Printf("First: '%s'\n", firstPerson.Props["name"].(string))
		secondPerson := record.Values[1].(neo4j.Node)
		fmt.Printf("Second: '%s'\n", secondPerson.Props["name"].(string))
	}

	// Now read the created persons. By using ExecuteRead method a connection
	// to a read replica can be used which reduces load on writer nodes in cluster.
	_, err = session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Code within this function might be invoked more than once in case of
		// transient errors.
		readPersonByName := `
			MATCH (p:Person)
			WHERE p.name = $person_name
			RETURN p.name AS name`
		result, err := tx.Run(ctx, readPersonByName, map[string]any{
			"person_name": "Alice",
		})
		if err != nil {
			return nil, err
		}
		// Iterate over the result within the transaction instead of using
		// Collect (just to show how it looks...). Result.Next returns true
		// while a record could be retrieved, in case of error result.Err()
		// will return the error.
		for result.Next(ctx) {
			fmt.Printf("Person name: '%s' \n", result.Record().Values[0].(string))
		}
		// Again, return any error back to driver to indicate rollback and
		// retry in case of transient error.
		return nil, result.Err()
	})
	if err != nil {
		panic(err)
	}
}
