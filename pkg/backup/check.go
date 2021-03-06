// Copyright 2020 PingCAP, Inc. Licensed under Apache-2.0.

package backup

import (
	"encoding/hex"

	"github.com/google/btree"
	"github.com/pingcap/log"
	"go.uber.org/zap"

	"github.com/Orion7r/pr/pkg/rtree"
)

// checkDupFiles checks if there are any files are duplicated.
func checkDupFiles(rangeTree *rtree.RangeTree) {
	// Name -> SHA256
	files := make(map[string][]byte)
	rangeTree.Ascend(func(i btree.Item) bool {
		rg := i.(*rtree.Range)
		for _, f := range rg.Files {
			old, ok := files[f.Name]
			if ok {
				log.Error("dup file",
					zap.String("Name", f.Name),
					zap.String("SHA256_1", hex.EncodeToString(old)),
					zap.String("SHA256_2", hex.EncodeToString(f.Sha256)),
				)
			} else {
				files[f.Name] = f.Sha256
			}
		}
		return true
	})
}
