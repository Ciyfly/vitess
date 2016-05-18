// Copyright 2014, Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vtgate

import (
	"github.com/youtube/vitess/go/sqltypes"
	"github.com/youtube/vitess/go/vt/topo"
	"golang.org/x/net/context"

	topodatapb "github.com/youtube/vitess/go/vt/proto/topodata"
)

var routerVSchema = `
{
	"sharded": true,
	"vindexes": {
		"user_index": {
			"type": "hash"
		},
		"music_user_map": {
			"type": "lookup_hash_unique",
			"owner": "music",
			"params": {
				"Table": "music_user_map",
				"From": "music_id",
				"To": "user_id"
			}
		},
		"name_user_map": {
			"type": "lookup_hash",
			"owner": "user",
			"params": {
				"Table": "name_user_map",
				"From": "name",
				"To": "user_id"
			}
		},
		"idx1": {
			"type": "hash"
		},
		"idx_noauto": {
			"type": "hash",
			"owner": "noauto_table"
		},
		"keyspace_id": {
			"type": "numeric"
		}
	},
	"tables": {
		"user": {
			"col_vindexes": [
				{
					"col": "Id",
					"name": "user_index"
				},
				{
					"col": "name",
					"name": "name_user_map"
				}
			],
			"autoinc": {
				"col": "id",
				"sequence": "user_seq"
			}
		},
		"user_extra": {
			"col_vindexes": [
				{
					"col": "user_id",
					"name": "user_index"
				}
			]
		},
		"music": {
			"col_vindexes": [
				{
					"col": "user_id",
					"name": "user_index"
				},
				{
					"col": "id",
					"name": "music_user_map"
				}
			],
			"Autoinc" : {
				"col": "id",
				"sequence": "user_seq"
			}
		},
		"music_extra": {
			"col_vindexes": [
				{
					"col": "user_id",
					"name": "user_index"
				},
				{
					"col": "music_id",
					"name": "music_user_map"
				}
			]
		},
		"music_extra_reversed": {
			"col_vindexes": [
				{
					"col": "music_id",
					"name": "music_user_map"
				},
				{
					"col": "user_id",
					"name": "user_index"
				}
			]
		},
		"noauto_table": {
			"col_vindexes": [
				{
					"col": "id",
					"name": "idx_noauto"
				}
			]
		},
		"ksid_table": {
			"col_vindexes": [
				{
					"col": "keyspace_id",
					"name": "keyspace_id"
				}
			]
		}
	}
}
`
var badVSchema = `
{
	"sharded": false,
	"tables": {
		"sharded_table": {}
	}
}
`

var unshardedVSchema = `
{
	"sharded": false,
	"tables": {
		"user_seq": {
			"type": "Sequence"
		},
		"music_user_map": {},
		"name_user_map": {}
	}
}
`

func createRouterEnv() (router *Router, sbc1, sbc2, sbclookup *sandboxConn) {
	cell := "aa"
	hc := newFakeHealthCheck()
	s := createSandbox("TestRouter")
	s.VSchema = routerVSchema
	sbc1 = &sandboxConn{}
	sbc2 = &sandboxConn{}
	hc.addTestTablet(cell, "-20", 1, "TestRouter", "-20", topodatapb.TabletType_MASTER, true, 1, nil, sbc1)
	hc.addTestTablet(cell, "40-60", 1, "TestRouter", "40-60", topodatapb.TabletType_MASTER, true, 1, nil, sbc2)

	createSandbox(KsTestUnsharded)
	sbclookup = &sandboxConn{}
	hc.addTestTablet(cell, "0", 1, KsTestUnsharded, "0", topodatapb.TabletType_MASTER, true, 1, nil, sbclookup)

	bad := createSandbox("TestBadSharding")
	bad.VSchema = badVSchema

	getSandbox(KsTestUnsharded).VSchema = unshardedVSchema

	serv := new(sandboxTopo)
	scatterConn := NewScatterConn(hc, topo.Server{}, serv, "", cell, 10, nil)
	router = NewRouter(context.Background(), serv, cell, "", scatterConn)
	return router, sbc1, sbc2, sbclookup
}

func routerExec(router *Router, sql string, bv map[string]interface{}) (*sqltypes.Result, error) {
	return router.Execute(context.Background(),
		sql,
		bv,
		"",
		topodatapb.TabletType_MASTER,
		nil,
		false)
}

func routerStream(router *Router, sql string) (qr *sqltypes.Result, err error) {
	results := make(chan *sqltypes.Result, 10)
	err = router.StreamExecute(context.Background(), sql, nil, "", topodatapb.TabletType_MASTER, func(qr *sqltypes.Result) error {
		results <- qr
		return nil
	})
	close(results)
	if err != nil {
		return nil, err
	}
	first := true
	for r := range results {
		if first {
			qr = &sqltypes.Result{Fields: r.Fields}
			first = false
		}
		qr.Rows = append(qr.Rows, r.Rows...)
		qr.RowsAffected += r.RowsAffected
	}
	return qr, nil
}
