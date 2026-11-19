package server_test

import (
	"context"
	"testing"

	"github.com/goccy/bigquery-emulator/server"
	"github.com/goccy/bigquery-emulator/types"
	bigqueryv2 "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/option"
)

// A table created with a materialized view definition must report
// type MATERIALIZED_VIEW from tables.get and tables.list, matching
// real BigQuery. Regression test for
// https://github.com/Recidiviz/bigquery-emulator/issues/76
func TestMaterializedViewReportsMaterializedViewType(t *testing.T) {
	const (
		projectID = "test"
		datasetID = "dataset1"
		mvID      = "mv"
		tableID   = "plain_table"
	)

	ctx := context.Background()

	bqServer, err := server.New(server.TempStorage)
	if err != nil {
		t.Fatal(err)
	}
	project := types.NewProject(projectID, types.NewDataset(datasetID))
	if err := bqServer.Load(server.StructSource(project)); err != nil {
		t.Fatal(err)
	}

	testServer := bqServer.TestServer()
	defer func() {
		testServer.Close()
		bqServer.Close()
	}()

	bqService, err := bigqueryv2.NewService(
		ctx,
		option.WithEndpoint(testServer.URL),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatal(err)
	}

	mvToInsert := &bigqueryv2.Table{
		TableReference: &bigqueryv2.TableReference{
			ProjectId: projectID,
			DatasetId: datasetID,
			TableId:   mvID,
		},
		MaterializedView: &bigqueryv2.MaterializedViewDefinition{
			Query: "SELECT 1 AS id",
		},
	}
	if _, err := bqService.Tables.Insert(projectID, datasetID, mvToInsert).Context(ctx).Do(); err != nil {
		t.Fatalf("failed to insert materialized view via v2 API: %v", err)
	}

	retrieved, err := bqService.Tables.Get(projectID, datasetID, mvID).Context(ctx).Do()
	if err != nil {
		t.Fatalf("failed to get materialized view: %v", err)
	}
	if retrieved.Type != "MATERIALIZED_VIEW" {
		t.Errorf("materialized view type = %q, want %q", retrieved.Type, "MATERIALIZED_VIEW")
	}
	if retrieved.MaterializedView == nil || retrieved.MaterializedView.Query != "SELECT 1 AS id" {
		t.Errorf("materialized view definition not preserved: %+v", retrieved.MaterializedView)
	}

	list, err := bqService.Tables.List(projectID, datasetID).Context(ctx).Do()
	if err != nil {
		t.Fatalf("failed to list tables: %v", err)
	}
	found := false
	for _, entry := range list.Tables {
		if entry.TableReference.TableId == mvID {
			found = true
			if entry.Type != "MATERIALIZED_VIEW" {
				t.Errorf("tables.list type for materialized view = %q, want %q", entry.Type, "MATERIALIZED_VIEW")
			}
		}
	}
	if !found {
		t.Errorf("materialized view %q not found in tables.list", mvID)
	}

	// Control: a plain table must still report type TABLE.
	plainToInsert := &bigqueryv2.Table{
		TableReference: &bigqueryv2.TableReference{
			ProjectId: projectID,
			DatasetId: datasetID,
			TableId:   tableID,
		},
		Schema: &bigqueryv2.TableSchema{
			Fields: []*bigqueryv2.TableFieldSchema{
				{Name: "id", Type: "INTEGER", Mode: "NULLABLE"},
			},
		},
	}
	if _, err := bqService.Tables.Insert(projectID, datasetID, plainToInsert).Context(ctx).Do(); err != nil {
		t.Fatalf("failed to insert plain table via v2 API: %v", err)
	}
	plain, err := bqService.Tables.Get(projectID, datasetID, tableID).Context(ctx).Do()
	if err != nil {
		t.Fatalf("failed to get plain table: %v", err)
	}
	if plain.Type != "TABLE" {
		t.Errorf("plain table type = %q, want %q", plain.Type, "TABLE")
	}
}
