package db

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
)

func NewDriver(cfg config.Config) (neo4j.DriverWithContext, error) {
	driver, err := neo4j.NewDriverWithContext(cfg.Neo4jURI, neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""))
	if err != nil {
		return nil, err
	}
	return driver, nil
}

func VerifyConnectivity(ctx context.Context, driver neo4j.DriverWithContext) error {
	return driver.VerifyConnectivity(ctx)
}

func AssertSchemaVersion(ctx context.Context, driver neo4j.DriverWithContext, required int) error {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	verAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, "MATCH (s:SchemaMeta {key:'schema'}) RETURN coalesce(s.current_version,0) AS v", nil)
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return 0, fmt.Errorf("SchemaMeta missing: run migrations")
		}
		v, _ := res.Record().Get("v")
		return v, nil
	})
	if err != nil {
		return err
	}

	ver, ok := verAny.(int64)
	if !ok {
		// Neo4j returns integer as int64
		return fmt.Errorf("unexpected schema version type: %T", verAny)
	}
	if int(ver) < required {
		return fmt.Errorf("schema version %d < required %d", ver, required)
	}
	return nil
}
