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

import "errors"

var (
	INVALID_FILE_FORMAT = errors.New("Not a valid compound file")
	READ_ERROR          = errors.New("Error reading compound file")
	OOXML_FILE_FORMAT   = errors.New("The supplied data appears to be in the Office 2007+ XML.")
	BLOCK_ERROR         = errors.New("Unable to read Data Block")
	EMPTY_BLOCK_ERROR   = errors.New("Can not return empty data")

	SMALLER_BIG_BLOCK_SIZE_DETAILS = NewBigBlockSize(SMALLER_BIG_BLOCK_SIZE, 9)
	LARGER_BIG_BLOCK_SIZE_DETAILS  = NewBigBlockSize(LARGER_BIG_BLOCK_SIZE, 12)
	/** The first 4 bytes of an OOXML file, used in detection */
	OOXML_FILE_HEADER = [4]byte{0x50, 0x4B, 0x03, 0x04}
)

const (
	SIGNATURE              uint64 = 0xE11AB1A1E011CFD0
	SMALLER_BIG_BLOCK_SIZE        = 0x0200

	LARGER_BIG_BLOCK_SIZE = 0x1000

	SMALL_BLOCK_SIZE = 0x0040
	PROPERTY_SIZE    = 0x0080 //How big single property is
	/**
	 * The minimum size of a document before it's stored using
	 *  Big Blocks (normal streams). Smaller documents go in the
	 *  Mini Stream (SBAT / Small Blocks)
	 */
	BIG_BLOCK_MINIMUM_DOCUMENT_SIZE = 0x1000
	/** The highest sector number you're allowed, 0xFFFFFFFA */
	LARGEST_REGULAR_SECTOR_NUMBER = -5

	/** Indicates the sector holds a DIFAT block (0xFFFFFFFC) */
	DIFAT_SECTOR_BLOCK = -4
	/** Indicates the sector holds a FAT block (0xFFFFFFFD) */
	FAT_SECTOR_BLOCK = -3
	/** Indicates the sector is the end of a chain (0xFFFFFFFE) */
	END_OF_CHAIN = -2
	/** Indicates the sector is not used (0xFFFFFFFF) */
	UNUSED_BLOCK = -1
)

const (
	BYTE_SIZE   = 1
	SHORT_SIZE  = 2
	INT_SIZE    = 4
	LONG_SIZE   = 8
	DOUBLE_SIZE = 8
)
