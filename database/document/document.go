package document

import (
	"context"
	"path"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/badger/v3/options"
	"github.com/genjidb/genji"
	"github.com/genjidb/genji/engine/badgerengine"
	"github.com/pingcap/log"
	"github.com/zhongzc/ng_monitoring/config"
	"github.com/zhongzc/ng_monitoring/utils"
	"go.uber.org/zap"
)

var documentDB *genji.DB
var closeCh chan struct{}

func Init(cfg *config.Config) {
	dataPath := path.Join(cfg.Storage.Path, "docdb")
	l, _ := simpleLogger(&cfg.Log)
	opts := badger.DefaultOptions(dataPath).
		WithCompression(options.ZSTD).
		WithZSTDCompressionLevel(3).
		WithBlockSize(8 * 1024).
		WithValueThreshold(128 * 1024).
		WithLogger(l)

	engine, err := badgerengine.NewEngine(opts)
	if err != nil {
		log.Fatal("failed to open a badger storage", zap.String("path", dataPath), zap.Error(err))
	}

	closeCh = make(chan struct{})
	go utils.GoWithRecovery(func() {
		doGCLoop(engine.DB, closeCh)
	}, nil)

	db, err := genji.New(context.Background(), engine)
	if err != nil {
		log.Fatal("failed to open a document database", zap.String("path", dataPath), zap.Error(err))
	}
	documentDB = db
}

func doGCLoop(db *badger.DB, closed chan struct{}) {
	log.Info("badger start to run value log gc loop")
	ticker := time.NewTicker(1 * time.Minute)
	defer func() {
		ticker.Stop()
		log.Info("badger stop running value log gc loop")
	}()
	for {
		select {
		case <-ticker.C:
			runValueLogGC(db)
		case <-closed:
			return
		}
	}
}

func runValueLogGC(db *badger.DB) {
	defer func() {
		r := recover()
		if r != nil {
			log.Error("panic when run badger value log",
				zap.Reflect("r", r),
				zap.Stack("stack trace"))
		}
	}()
	err := db.RunValueLogGC(0.5)
	if err == nil {
		log.Info("badger run value log gc success")
	} else if err != badger.ErrNoRewrite {
		log.Error("badger run value log gc failed", zap.Error(err))
	}
}

func Get() *genji.DB {
	return documentDB
}

func Stop() {
	close(closeCh)
	if err := documentDB.Close(); err != nil {
		log.Fatal("failed to close the document database", zap.Error(err))
	}
}
