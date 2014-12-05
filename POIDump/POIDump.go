// Developer: Ali Naqvi
//
// This program or package and any associated files are licensed under the
// Apache License, Version 2.0 (the "License"); you may not use these files
// except in compliance with the License. You can get a copy of the License
// at: http://www.apache.org/licenses/LICENSE-2.0.
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"poi4go/poifs"
	"strings"
	"unicode"
)

func main() {
	inpFile := flag.String("i", "", "Input file <MS Office file(.xls|.doc|.mpp...)>")
	dumpToScreen := flag.Bool("s", false, "Dump contents to screen")
	flag.Parse()
	if len(*inpFile) == 0 {
		flag.Usage()
		os.Exit(1)
	}
	file, err := os.Open(*inpFile)
	if err != nil {
		fmt.Printf("Error encountered: %v\n", err)
		os.Exit(1)
	}
	fs, err := poifs.FileSystemFromReader(file)
	if err != nil {
		fmt.Printf("Error encountered: %v\n", err)
		os.Exit(1)
	}
	var root poifs.DirectoryEntry
	root, err = fs.GetRoot()
	if err != nil {
		fmt.Printf("Error encountered: %v\n", err)
		os.Exit(1)
	}
	if *dumpToScreen {
		dumpScreen(root, root.GetName())
	} else {
		wd, _ := os.Getwd()
		_path := path.Join(wd, root.GetName())

		err = os.MkdirAll(_path, 0777)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		dump(root, _path)
	}
}

func dump(root poifs.DirectoryEntry, parent string) {
	entries := root.Entries()
	for _, entry := range entries {
		switch node := entry.(type) {
		case *poifs.DocumentNode:
			name := strings.Trim(node.GetName(), " ")
			sLen := 0
			if !unicode.IsPrint(rune(name[0])) {
				sLen = 1
			}
			name = string(name[sLen:])
			fileName := path.Join(parent, name)

			bytes := make([]byte, node.GetSize())
			is := poifs.NewDocInputStreamFromDocNode(node)
			_, err := io.ReadFull(is, bytes)
			if err != nil {
				panic(err)
			}
			is.Close()

			file, err := os.Create(fileName)
			if err != nil {
				panic(err)
			}
			file.Write(bytes)
			file.Close()

		case *poifs.DirectoryNode:
			name := strings.Trim(node.GetName(), " ")
			sLen := 0
			if !unicode.IsPrint(rune(name[0])) {
				sLen = 1
			}
			name = string(name[sLen:])
			fileName := path.Join(parent, name)

			//fileName := path.Join(parent, node.GetName())

			err := os.MkdirAll(fileName, 0777)
			if err != nil {
				panic(err)
			}
			dump(node, fileName)
		default:
			fmt.Printf("Skipping unsupported POIFS entry: %v", entry)
		}
	}
}

func dumpScreen(root poifs.DirectoryEntry, parent string) {
	entries := root.Entries()
	for _, entry := range entries {
		switch node := entry.(type) {
		case *poifs.DocumentNode:
			fileName := path.Join(parent, strings.Trim(node.GetName(), " "))
			fmt.Printf("Reading Doc: %s\n", fileName)

			bytes := make([]byte, node.GetSize())
			is := poifs.NewDocInputStreamFromDocNode(node)
			_, err := io.ReadFull(is, bytes)
			if err != nil {
				panic(err)
			}
			is.Close()

		case *poifs.DirectoryNode:
			fileName := path.Join(parent, node.GetName()) //  strings.Trim(node.GetName(), " "))
			fmt.Printf("Making folder: %s\n", fileName)

			dumpScreen(node, fileName)
		default:
			fmt.Printf("Skipping unsupported POIFS entry: %v", entry)
		}
	}
}
