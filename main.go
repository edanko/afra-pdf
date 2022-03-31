package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/ledongthuc/pdf"
)

var (
	partNameRe                 = regexp.MustCompile(`(\w{4}-\w+-\w+)\/\w+\/`)
	errPageProfile             = errors.New("profile")
	errPageProfileBendingTable = errors.New("profile bending table")
	errPageMarkingPlan         = errors.New("block marking type")
	errPageInComing            = errors.New("in-coming")
	errPageOutGoing            = errors.New("out-going")
)

const (
	byNcName = false
)

func isSkip(e error) bool {
	switch e {
	case errPageProfile,
		errPageProfileBendingTable,
		errPageMarkingPlan,
		errPageInComing,
		errPageOutGoing:
		return true
	default:
		return false
	}
}

func main() {
	if len(os.Args) < 2 {
		log.Fatalln("[x] no input file")
	}

	fname := filepath.Clean(os.Args[1])
	dxfFiles := walk("dxf")

	f, r, err := pdf.Open(fname)
	if err != nil {
		log.Fatalln("[x] error opening pdf:", err)
	}
	defer f.Close()

	wg := &sync.WaitGroup{}
	for pageIdx := 1; pageIdx <= r.NumPage(); pageIdx++ {
		wg.Add(1)
		go func(pageIdx int) {
			defer wg.Done()
			materialMapName, foundParts, err := processPage(r, pageIdx)

			if err != nil {
				if isSkip(err) {
					fmt.Printf("[i] page %d skipped: %s\n", pageIdx, err)
				} else {
					log.Fatalln(err)
				}
			}

			err = out(dxfFiles, materialMapName, foundParts)
			if err != nil {
				log.Fatalln(err)
			}
		}(pageIdx)
	}
	wg.Wait()
}

func out(dxf map[string]string, materialMapName string, parts []string) error {
	outDir := filepath.Join("out", materialMapName)

	err := os.MkdirAll(outDir, 0755)
	if err != nil {
		return err
	}

	for _, part := range parts {
		var found bool
		if path, ok := dxf[part]; ok {
			out := filepath.Join(outDir, filepath.Base(path))

			fmt.Printf("[c] %s -> %s\n", part, filepath.Join(materialMapName, filepath.Base(path)))
			err = copyFile(path, out)
			if err != nil {
				return err
			}
			found = true
		} else {
			for _, m := range []string{"BA", "BB", "BC"} {
				b := []byte(part)

				b[5] = m[0]
				b[6] = m[1]

				if path, ok := dxf[string(b)]; ok {
					out := filepath.Join(outDir, filepath.Base(path))
					fmt.Printf("[r] %s (%s) -> %s\n", string(b), part, filepath.Join(materialMapName, filepath.Base(path)))
					err = copyFile(path, out)
					if err != nil {
						return err
					}
					found = true
				}
			}
		}
		if !found {
			fmt.Printf("[n] %s not found\n", part)
		}
	}
	return nil
}

func processPage(r *pdf.Reader, pageIdx int) (string, []string, error) {
	page := r.Page(pageIdx)

	var materialMapName string

	pt, err := page.GetPlainText(nil)
	if err != nil {
		return "", []string{}, err
	}

	if strings.Contains(pt, "END A") {
		return "", []string{}, errPageProfile
	}

	if strings.Contains(pt, "MARKING PLAN") {
		return "", []string{}, errPageMarkingPlan
	}

	if strings.Contains(pt, "IN - COMING") {
		return "", []string{}, errPageInComing
	}

	if strings.Contains(pt, "OUT - GOING") {
		return "", []string{}, errPageOutGoing
	}

	if strings.Contains(pt, "BENDING TABLE") {
		return "", []string{}, errPageProfileBendingTable
	}

	/*for _, v := range page.Content().Text {
		fmt.Println(v.S, v.X, v.Y)
	}

	os.Exit(1)*/

	if byNcName {
		var lasty float64
		for _, v := range page.Content().Text {

			x := math.Round(v.X*1000) / 1000

			switch x {
			case 73.800:
				if v.Y-lasty > 2 {
					lasty = v.Y
					materialMapName += v.S
				}
			}
		}
	} else {
		var lasty float64
		for _, v := range page.Content().Text {

			x := math.Round(v.X*1000) / 1000

			switch x {
			case 520.560:
				if v.Y > 200 {
					continue
				}
				fallthrough
			case 736.735:
				if v.Y-lasty > 2 {
					lasty = v.Y
					materialMapName += v.S
				}
			}
		}
	}

	if materialMapName == "" {
		materialMapName = "unknown"
	}

	return materialMapName, getPartsIds(pt), nil
}

func getPartsIds(in string) []string {
	m := partNameRe.FindAllStringSubmatch(in, -1)

	var allParts []string
	for _, v := range m {
		allParts = append(allParts, v[1])
	}

	return uniq(allParts)
}

func walk(path string) map[string]string {
	files := make(map[string]string)

	err := fs.WalkDir(os.DirFS(path), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Fatal(err)
		}

		if d.IsDir() {
			return nil
		}

		b := filepath.Base(path)
		if filepath.Ext(b) != ".dxf" {
			return nil
		}

		// skip bending templates
		if b[:2] == "fp" || b[:5] == "templ" {
			return nil
		}

		p := strings.TrimSuffix(b, filepath.Ext(b))
		ss := strings.Split(p, "-")
		ss = ss[len(ss)-3:]
		p = strings.Join(ss, "-")

		files[p] = path

		return nil
	})

	if err != nil {
		log.Fatalln(err)
	}
	return files
}

func uniq(in []string) []string {
	u := make([]string, 0, len(in))
	m := make(map[string]bool)

	for _, val := range in {
		if _, ok := m[val]; !ok {
			m[val] = true
			u = append(u, val)
		}
	}

	return u
}

func copyFile(src, dest string) error {
	if _, err := os.Stat(dest); errors.Is(err, fs.ErrExist) {
		return nil
	}

	from, err := os.Open(src)
	if err != nil {
		return err
	}
	defer from.Close()

	to, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer to.Close()

	_, err = io.Copy(to, from)
	return err
}
