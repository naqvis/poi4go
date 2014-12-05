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

import "io"

var (
	empty_big_block_array   = make([]*DocumentBlock, 0)
	empty_small_block_array = make([]*smallDocumentBlock, 0)
)

type smallBlockStore struct {
	smallBlocks  []*smallDocumentBlock
	path         POIFSDocumentPath
	name         string
	size         int
	bigBlocksize POIFSBigBlockSize
}

type bigBlockStore struct {
	bigBlocks    []*DocumentBlock
	path         POIFSDocumentPath
	name         string
	size         int
	bigBlocksize POIFSBigBlockSize
}

func createSmallBlockStore(bigBlocksize POIFSBigBlockSize, blocks []*smallDocumentBlock) *smallBlockStore {
	sb := new(smallBlockStore)
	sb.bigBlocksize = bigBlocksize
	sb.smallBlocks = make([]*smallDocumentBlock, len(blocks))
	copy(sb.smallBlocks, blocks)
	sb.size = -1
	return sb
}

func newSmallBlockStore(bigBlogSize POIFSBigBlockSize, path POIFSDocumentPath, name string, size int) *smallBlockStore {
	sb := new(smallBlockStore)
	sb.smallBlocks = make([]*smallDocumentBlock, 0)
	sb.path = path
	sb.name = name
	sb.size = size
	return sb
}

func (sb *smallBlockStore) isValid() bool {
	return len(sb.smallBlocks) > 0
}

func createBigBlockStore(bigBlocksize POIFSBigBlockSize, blocks []*DocumentBlock) *bigBlockStore {
	sb := new(bigBlockStore)
	sb.bigBlocksize = bigBlocksize
	sb.bigBlocks = make([]*DocumentBlock, len(blocks))
	copy(sb.bigBlocks, blocks)
	sb.size = -1
	return sb
}

func newBigBlockStore(bigBlogSize POIFSBigBlockSize, path POIFSDocumentPath, name string, size int) *bigBlockStore {
	sb := new(bigBlockStore)
	sb.bigBlocks = make([]*DocumentBlock, 0)
	sb.path = path
	sb.name = name
	sb.size = size
	return sb
}

func (sb *bigBlockStore) isValid() bool {
	return len(sb.bigBlocks) > 0
}

func (sb *bigBlockStore) WriteBlocks(stream io.Writer) error {
	for _, b := range sb.bigBlocks {
		err := b.WriteData(stream)
		if err != nil {
			return err
		}
	}
	return nil
}

func (sb *bigBlockStore) CountBlocks() int {
	if sb.isValid() {
		return len(sb.bigBlocks)
	}
	return 0
}
