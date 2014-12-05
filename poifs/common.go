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
	"errors"
	"hash/fnv"
)

type POIFSBigBlockSize struct {
	BigBlockSize int
	HeaderValue  int16
}

func NewBigBlockSize(blocksize int, headval int16) POIFSBigBlockSize {
	return POIFSBigBlockSize{blocksize, headval}
}

func (b POIFSBigBlockSize) PropertiesPerBlock() int {
	return b.BigBlockSize / PROPERTY_SIZE
}

func (b POIFSBigBlockSize) BATEntriesPerBlock() int {
	return b.BigBlockSize / INT_SIZE
}

func (b POIFSBigBlockSize) XBATEntriesPerBlock() int {
	return b.BATEntriesPerBlock() - 1
}

func (b POIFSBigBlockSize) NextXBATChainOffset() int {
	return b.XBATEntriesPerBlock() * INT_SIZE
}

func fillArray(a []int, val int) {
	for i, _ := range a {
		a[i] = val
	}
}

func hashCode(str string) int {
	hasher := fnv.New32()
	hasher.Write([]byte(str))
	return int(hasher.Sum32())
}

func getInt(data []byte, offset int) int {
	i := offset
	b0 := int32(data[i] & 0xFF)
	i += 1
	b1 := int32(data[i] & 0xFF)
	i += 1
	b2 := int32(data[i] & 0xFF)
	i += 1
	b3 := int32(data[i] & 0xFF)
	return int((b3 << 24) + (b2 << 16) + (b1 << 8) + (b0 << 0))
}

func putInt(data []byte, offset, value int) {
	i := offset
	data[i] = (byte)((value >> 0) & 0xFF)
	i += 1
	data[i] = (byte)((value >> 8) & 0xFF)
	i += 1
	data[i] = (byte)((value >> 16) & 0xFF)
	i += 1
	data[i] = (byte)((value >> 24) & 0xFF)
}

func putShort(data []byte, offset int, value int16) {
	i := offset
	data[i] = (byte)((value >> 0) & 0xFF)
	i += 1
	data[i] = (byte)((value >> 8) & 0xFF)
}

func getShort(data []byte, offset int) int16 {
	b0 := data[offset] & 0xFF
	b1 := data[offset+1] & 0xFF
	return (int16)((b1 << 8) + (b0 << 0))
}

func min(a, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}

type Stack []interface{}

func (stack *Stack) Pop() (interface{}, error) {
	theStack := *stack
	if len(theStack) == 0 {
		return nil, errors.New("can't Pop() an empty stack")
	}
	x := theStack[len(theStack)-1]
	*stack = theStack[:len(theStack)-1]
	return x, nil
}

func (stack *Stack) Push(x interface{}) {
	*stack = append(*stack, x)
}

func (stack Stack) Top() (interface{}, error) {
	if len(stack) == 0 {
		return nil, errors.New("can't Top() an empty stack")
	}
	return stack[len(stack)-1], nil
}

func (stack Stack) Cap() int {
	return cap(stack)
}

func (stack Stack) Len() int {
	return len(stack)
}

func (stack Stack) IsEmpty() bool {
	return len(stack) == 0
}

type Iterator interface {
	HasNext() bool
	Next() interface{}
}
