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
	"os"
	"poi4go/poifs"
)

func main() {
	inpFile := flag.String("i", "", "Input file <MS Office file(.xls|.doc|.mpp...)>")
	outFile := flag.String("o", "", "Output file <MS Office file(.xls|.doc|.mpp...)>")
	flag.Parse()
	if len(*inpFile) == 0 || len(*outFile) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	file, err := os.Open(*inpFile)
	if err != nil {
		fmt.Printf("Error encountered: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()
	fs, err := poifs.FileSystemFromReader(file)
	if err != nil {
		fmt.Printf("Error encountered: %v\n", err)
		os.Exit(1)
	}

	out, err := os.Create(*outFile)
	if err != nil {
		fmt.Printf("Error encountered: %v\n", err)
		os.Exit(1)
	}
	defer out.Close()
	err = fs.WriteFileSystem(out)
	if err != nil {
		fmt.Printf("Error encountered in writing: %v\n", err)
	} else {
		fmt.Println("File written successfully.")
	}
}
