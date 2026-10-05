// atlas-index is a read-only source scanner; its only writes are to its own SQLite database.
package main

import (
	"ai-atlas/internal/atlas"
	"encoding/json"
	"flag"
	"log"
	"os"
)

func main() {
	home := flag.String("home", "", "Codex home")
	db := flag.String("db", "", "Index database")
	temps := flag.Bool("temps", false, "Also scan temporary files")
	flag.Parse()
	s, err := atlas.New(*home, *db)
	if err != nil {
		log.Fatal(err)
	}
	defer atlas.Close(s)
	if err = atlas.Scan(s, *temps); err != nil {
		log.Fatal(err)
	}
	v, err := s.Overview("", "")
	if err != nil {
		log.Fatal(err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err = enc.Encode(v); err != nil {
		log.Fatal(err)
	}
	for _, e := range s.Status().Errors {
		log.Print(e)
	}
}
