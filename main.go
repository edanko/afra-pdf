package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ledongthuc/pdf"
)

var re = regexp.MustCompile(`(\w{4}-\w+-\w+)\/`)
var thicknessRE = regexp.MustCompile(`(.{4})x\w+x\w+`)
var section string

func main() {
	if len(os.Args) < 2 {
		log.Fatalln("[x] no input file")
	}

	fname := os.Args[1]
	fname = filepath.Clean(fname)

	spl := strings.Split(fname, " ")
	section = strings.Replace(spl[len(spl)-1], ".pdf", "", 1)

	fmt.Println("[i] section:", section)

	dxfFiles := walk("dxf")

	f, r, err := pdf.Open(fname)
	if err != nil {
		log.Fatalln("[x] error opening pdf:", err)
	}
	defer f.Close()

	for pageIdx := 1; pageIdx <= r.NumPage(); pageIdx++ {
		materialMapName, foundParts, thickness, err := processPage(r, pageIdx)

		if err != nil {
			fmt.Println("[i] skipping page", pageIdx)
			continue
		}
		// by thickness
		out(dxfFiles, thickness, foundParts)
		// by material map
		out(dxfFiles, materialMapName, foundParts)
	}
}

func out(dxf map[string]string, materialMapName string, parts []string) {
	workingDir, _ := os.Getwd()
	outDir := filepath.Join(workingDir, section, materialMapName)

	if len(materialMapName) == 4 {
		outDir = filepath.Join(workingDir, section, "_by thickness", materialMapName)
	}

	err := os.MkdirAll(outDir, 0755)
	if err != nil {
		log.Fatalln("[x] can't create dir:", err)
	}

	for file, path := range dxf {
		for _, part := range parts {
			if !strings.Contains(file, part) {
				continue
			}

			out := filepath.Join(outDir, filepath.Base(path))

			fmt.Printf("[i] copying %s -> %s\n", part, materialMapName+string(filepath.Separator)+filepath.Base(path))
			copy(filepath.Join(workingDir, path), out)
		}
	}

}

func processPage(r *pdf.Reader, pageIdx int) (string, []string, string, error) {
	page := r.Page(pageIdx)

	var materialMapName string

	// skip profile pages
	var text string
	for _, t := range page.Content().Text {
		text += strings.ToUpper(t.S)
	}

	if strings.Contains(text, "END A") {
		return "", []string{}, "", fmt.Errorf("profile")
	}

	if strings.Contains(text, "MARKING PLAN") {
		return "", []string{}, "", fmt.Errorf("block marking plan")
	}

	if strings.Contains(text, "IN - COMING") {
		return "", []string{}, "", fmt.Errorf("in-coming")
	}

	if strings.Contains(text, "OUT - GOING") {
		return "", []string{}, "", fmt.Errorf("out-going")
	}

	for _, v := range page.Content().Text {
		if v.X == 736.7345893872 && v.Y == 189.36515149200002 {
			materialMapName += v.S
		}

		if v.X == 736.6305893040001 && v.Y == 187.3391498712 {
			materialMapName += v.S
		}

		if v.X == 736.7340893868001 && v.Y == 189.36540149220002 {
			materialMapName += v.S
		}

		if v.X == 520.5597917759999 {
			materialMapName += v.S
		}
		if v.X == 522.479791008 {
			materialMapName += v.S
		}
	}

	materialMapName = strings.TrimSuffix(materialMapName, "Y")
	materialMapName = strings.Trim(materialMapName, " ")

	// skip profile pages
	if materialMapName == "" {

		/* for _, v := range page.Content().Text {
			fmt.Println(v.S, v.X, v.Y)
		} */

		log.Fatalf("[x] material map name not found on page %d\n", pageIdx)
	}

	pt, err := page.GetPlainText(nil)
	if err != nil {
		log.Fatalln(err)
	}

	return materialMapName, getPartsIds(pt), getThickness(pt), nil
}

func getPartsIds(in string) []string {
	m := re.FindAllStringSubmatch(in, -1)

	var allParts []string
	for _, v := range m {
		allParts = append(allParts, v[1])
	}

	return uniq(allParts)
}

func getThickness(in string) string {
	m := thicknessRE.FindAllStringSubmatch(in, -1)

	return m[0][1]
}

func walk(path string) (files map[string]string) {

	files = make(map[string]string)

	walkfn := func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		b := filepath.Base(path)

		// skip bending templates
		if b[:2] == "fp" {
			return nil
		}

		p := strings.TrimSuffix(b, filepath.Ext(b))
		files[p] = path

		return nil
	}

	err := filepath.Walk(path, walkfn)
	if err != nil {
		log.Fatalln("[x] walk error", err)
	}
	return
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

func copy(src, dest string) {
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		fmt.Println(" -> file already exists, skipping...")
		return
	}

	from, err := os.Open(src)
	if err != nil {
		log.Fatal(err)
	}
	defer from.Close()

	to, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer to.Close()

	_, err = io.Copy(to, from)
	if err != nil {
		log.Fatal(err)
	}
}
