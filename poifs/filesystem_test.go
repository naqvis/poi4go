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

package poifs

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

const (
	test_data = "test_data/"
)

func openResource(t *testing.T, name string) io.Reader {
	ret, err := os.Open(test_data + name)
	if err != nil {
		t.Error("Unable to open file: " + test_data + name)
	}
	return ret
}

/**
 * Check that we do the right thing when the list of which
 *  sectors are BAT blocks points off the list of
 *  sectors that exist in the file.
 */

func TestFATandDIFATsectors(t *testing.T) {
	_, err := FileSystemFromReader(openResource(t, "ReferencesInvalidSectors.mpp"))
	if err == nil {

		t.Error("File is corrupt and shouldn't have been opened")
	}
	if !strings.HasPrefix(err.Error(), "Your file contains 695  sectors") {
		t.Errorf("failed with msg: %s", err.Error())
	}
}

/**
 * Tests that we can write and read a file that contains XBATs
 *  as well as regular BATs.
 * However, because a file needs to be at least 6.875mb big
 *  to have an XBAT in it, we don't have a test one. So, generate it.
 */

func TestBATandXBAT(t *testing.T) {

	fs := NewFileSystem()
	root, err := fs.GetRoot()
	if err != nil {
		t.Error(err)
	}
	hugeStream := make([]byte, 8*1024*1024)
	inp := bytes.NewBuffer(hugeStream)

	root.CreateDocument("BIG", inp)
	var buff bytes.Buffer

	fs.WriteFileSystem(&buff)
	fsData := buff.Bytes()

	header, err := NewHeaderBlockFromReader(&buff)
	if err != nil {
		t.Error(err)
	}

	if header.GetBATCount() != 109+21 {
		t.Errorf("Invalid BAT count. Expected %d, got %d", 109+21, header.GetBATCount())
	}
	if header.GetXBATCount() != 1 {
		t.Errorf("Invalid XBAT count. Expected %d, got %d", 1, header.GetXBATCount())
	}

	// We should have 21 BATs in the XBAT
	xbatData := make([]byte, 512)

	offset := (1 + header.GetXBATIndex()) * 512
	copy(xbatData[0:], fsData[offset:])
	var xbuff bytes.Buffer
	xbuff.Write(xbatData)
	xbat := CreateBATBlock(SMALLER_BIG_BLOCK_SIZE_DETAILS, xbuff)
	for i := 0; i < 21; i++ {
		val, err := xbat.GetValueAt(i)
		if err != nil || (val == UNUSED_BLOCK) {
			t.Error("shouldn't be UNUSED_BLOCK, %v", err)
		}
	}
	for i := 21; i < 127; i++ {
		val, err := xbat.GetValueAt(i)
		if err != nil || val != UNUSED_BLOCK {
			t.Errorf("val should be equal to UNUSED_BLOCK, %v", val)
		}
	}
	val, err := xbat.GetValueAt(127)
	if err != nil || val != END_OF_CHAIN {
		t.Errorf("val should be equal to END_OF_CHAIN, %v", err)
	}

	// Load the blocks and check with that

	blockList, err := RawDataBlockList(&buff, SMALLER_BIG_BLOCK_SIZE_DETAILS)
	if err != nil {
		t.Error(err)
	}

	if len(fsData)/512 != blockList.BlockCount()+1 { //Header not counted
		t.Errorf("block count not equals. expected %d, found %d",
			len(fsData)/512, blockList.BlockCount()+1)
	}

	NewBlockAllocationTableReader(header.GetBigBlockSize(), header.GetBATCount(),
		header.GetBATArray(), header.GetXBATCount(),
		header.GetXBATIndex(), blockList)

	if len(fsData)/512 != blockList.BlockCount()+1 { //Header not counted
		t.Errorf("block count not equals. expected %d, found %d",
			len(fsData)/512, blockList.BlockCount()+1)
	}

	// Now load it and check

	fs = nil
	tmp := bytes.NewBuffer(fsData)
	fs, err = FileSystemFromReader(tmp)
	if err != nil {
		t.Error(err)
	}

	root, err = fs.GetRoot()
	if err != nil {
		t.Error(err)
	}

	if 1 != root.EntryCount() {
		t.Errorf("Invalid entry count. Expected 1, found %d", root.EntryCount())
	}

	big, errr := root.Entry("BIG")
	if errr != nil {
		t.Error(errr)
	}

	size := big.(*DocumentNode).GetSize()
	if len(hugeStream) != size {
		t.Errorf("Invalid doc size. expected %d, found %d",
			len(hugeStream), size)
	}
}

/**
 * Most OLE2 files use 512byte blocks. However, a small number
 *  use 4k blocks. Check that we can open these.
 */

func assertEquals(t *testing.T, a, b int) {
	if a != b {
		t.Error("Assertion failed.")
	}
}

func assertTrue(t *testing.T, cond bool) {
	if !cond {
		t.Error("AssertTrue failed.")
	}
}

func Test4KBlocks(t *testing.T) {
	inp := openResource(t, "BlockSize4096.zvi")

	// First up, check that we can process the header properly
	header, err := NewHeaderBlockFromReader(inp)
	if err != nil {
		t.Error(err)
	}

	bigBlockSize := header.GetBigBlockSize()
	if 4096 != bigBlockSize.BigBlockSize {
		t.Errorf("Invalid block size. Expected %d, found %d",
			4096, bigBlockSize.BigBlockSize)
	}

	// Check the FAT info looks sane
	assertEquals(t, 1, len(header.GetBATArray()))
	assertEquals(t, 1, header.GetBATCount())
	assertEquals(t, 0, header.GetXBATCount())

	// Now check we can get the basic FAT
	_, err = RawDataBlockList(inp, bigBlockSize)
	if err != nil {
		t.Error(err)
	}

	// Now try and open properly
	fs, err := FileSystemFromReader(openResource(t, "BlockSize4096.zvi"))
	if err != nil {
		t.Error(err)
	}
	root, err := fs.GetRoot()
	if err != nil {
		t.Error(err)
	}

	assertTrue(t, root.EntryCount() > 3)
	// Check we can get at all the contents
	checkAllDirectoryContents(root)

	// Finally, check we can do a similar 512byte one too
	// Now try and open properly
	fs, err = FileSystemFromReader(openResource(t, "BlockSize512.zvi"))
	if err != nil {
		t.Error(err)
	}
	root, err = fs.GetRoot()
	if err != nil {
		t.Error(err)
	}

	assertTrue(t, root.EntryCount() > 3)
	// Check we can get at all the contents
	checkAllDirectoryContents(root)

}

func checkAllDirectoryContents(dir DirectoryEntry) {
	for _, entry := range dir.Entries() {
		switch entry.(type) {
		case DirectoryEntry:
			checkAllDirectoryContents(entry.(DirectoryEntry))
		case DocumentEntry:
			doc := entry.(*DocumentNode)
			dis := NewDocInputStreamFromDocNode(doc)
			numBytes := dis.Available()
			data := make([]byte, numBytes)
			_, err := dis.Read(data)
			if err != nil {
				panic(err)
			}
		}
	}
}

/**
 * Test that we can open files that come via Lotus notes.
 * These have a top level directory without a name....
 */
func TestNotesOLE2Files(t *testing.T) {
	fs, err := FileSystemFromReader(openResource(t, "Notes.ole2"))
	if err != nil {
		t.Error(err)
	}
	root, err := fs.GetRoot()
	if err != nil {
		t.Error(err)
	}

	// check the contents
	assertEquals(t, 1, root.EntryCount())

	entry := root.Entries()[0]
	assertTrue(t, entry.IsDirectory())
	dir, ok := entry.(DirectoryEntry)
	assertTrue(t, ok)

	// The directory lacks a name
	if len(dir.GetName()) != 0 {
		t.Errorf("Invalid name. Empty expected, found %s",
			dir.GetName())
	}

	// Has two children
	assertEquals(t, 2, dir.EntryCount())

	// Check them
	assertTrue(t, dir.Entries()[0].IsDocument())

	if !strings.EqualFold("Ole10Native", dir.Entries()[0].GetName()) {
		t.Errorf("Invalid name. Expected %s, found %s",
			"\u0001Ole10Native", dir.Entries()[0].GetName())
	}

	assertTrue(t, dir.Entries()[1].IsDocument())

	if !strings.EqualFold("CompObj", dir.Entries()[1].GetName()) {
		t.Errorf("Invalid name. Expected %s, found %s",
			"u0001CompObj", dir.Entries()[1].GetName())
	}

}
