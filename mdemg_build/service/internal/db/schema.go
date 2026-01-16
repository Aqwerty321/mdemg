package db

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func GetSchemaVersion(ctx context.Context, d neo4j.DriverWithContext) (int, error) {
	session := d.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rec, err := tx.Run(ctx, `MATCH (m:SchemaMeta {key:'schema'}) RETURN coalesce(m.current_version,0) AS v LIMIT 1`, nil)
		if err != nil {
			return 0, err
		}
		if !rec.Next(ctx) {
			return 0, fmt.Errorf("schema meta not found")
		}
		vAny, _ := rec.Record().Get("v")
		v, ok := vAny.(int64)
		if !ok {
			return 0, fmt.Errorf("unexpected schema version type: %T", vAny)
		}
		return int(v), nil
	})
	if err != nil {
		return 0, err
	}
	return res.(int), nil
}
