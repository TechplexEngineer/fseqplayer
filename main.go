package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/DataDog/zstd"
	"log"
	"os"
)

const (
	V2FSEQ_HEADER_SIZE = 32 //copied from https://github.com/smeighan/xLights/blob/master/xLights/FSEQFile.cpp#L675
)

//type CompressionType uint8
const (
	CompressionType_none = 0
	CompressionType_zstd = 1
	CompressionType_zlib = 2
)

type fseqHeader struct {
	//0-3 - file identifier, must be 'PSEQ'
	Magic [4]byte
	//4-5 - Offset to start of channel data
	OffsetToChanData uint16 //m_seqChanDataOffset
	//6   - minor version, should be 0
	MinorVersion uint8
	//7   - major version, should be 1 or 2
	MajorVersion uint8
}

// 1 - uint8
// 2 - uint16
// 4 - uint32

type fseqv2Header struct {
	//8-9 - standard header length/index to first variable header
	HeaderLen uint16
	//10-13 - channel count per frame (*)
	NumChannelsPerFrame uint32
	//14-17 - number of frames
	NumFrames uint32
	//18  - step time in ms, usually 25 or 50
	TimeStep uint8
	//19  - bit flags/reserved should be 0
	Flags uint8
	//20  - compression type 0 for uncompressed, 1 for zstd, 2 for libz/gzip
	CompressionType uint8
	//21  - number of compression blocks, 0 if uncompressed
	NumCompressedBlocks uint8 //maxBlocks
	//22  - number of sparse ranges, 0  if none
	NumSparseRanges uint8
	//23  - bit flags/reserved, unused right now, should be 0
	Flags2 uint8
	//24-31 - 64bit unique identifier, likely a timestamp or uuid
	Identifier uint64
}

//numberOfBlocks*8 - compress block index
type fseqv2_block struct {
	//0-3 - frame number
	FrameNum uint32
	//4-7 - length of block
	BlockLen uint32
}

//numberOfSparseRanges*6 - sparse range definitions
//type fseqv2_sparse struct {
//	//0-2 - start channel number
//	startChan
//	//3-5 - number of channels
//}

type block struct {
	BlockNum    uint32
	BlockOffset uint32
	BlockLen    uint32
}

func main() {
	//flag.String("file", )

	flag.Parse()

	fileName := flag.Arg(0)

	if len(fileName) <= 0 {
		panic("usage")
	}

	fp, err := os.Open(fileName)
	if err != nil {
		panic(err)
	}

	header := fseqHeader{}

	err = binary.Read(fp, binary.LittleEndian, &header)
	if err != nil {
		panic(err)
	}

	if header.MinorVersion != 0 {
		panic("Got unexpected minor version. only 2.0 is supported")
	}
	if header.MajorVersion != 2 {
		panic("got unexpected major version. only 2.0 is supported")
	}

	if string(header.Magic[:]) != "PSEQ" {
		panic("Expected PSEQ magic")
	}

	fmt.Printf("HEADER: %+v\n", header)

	v2hdr := fseqv2Header{}

	err = binary.Read(fp, binary.LittleEndian, &v2hdr)
	if err != nil {
		panic(err)
	}

	fmt.Printf("HEADER v2: %+v\n", v2hdr)

	if v2hdr.NumSparseRanges > 0 {
		panic("sparse ranges are unsupported")
	}

	// move the pointer to the end of the header
	_, err = fp.Seek(int64(V2FSEQ_HEADER_SIZE), 0)
	if err != nil {
		panic(err)
	}

	blocks := []block{}

	offset := uint32(header.OffsetToChanData)
	for blockNum := uint8(0); blockNum < v2hdr.NumCompressedBlocks; blockNum++ {
		blk := fseqv2_block{}
		err = binary.Read(fp, binary.LittleEndian, &blk)
		if err != nil {
			panic(err)
		}

		if blk.BlockLen > 0 {
			//fmt.Printf("IDX:%d \t %+v \t Start: %d \t End: %d\n", blockNum, blk, offset, offset+blk.BlockLen)
			blocks = append(blocks, block{
				BlockNum:    blk.FrameNum,
				BlockOffset: offset,
				BlockLen:    blk.BlockLen,
			})
			offset += blk.BlockLen
		} else {
			//block with zero length data
		}
	}
	//                                             why is this +2 ?
	//fmt.Printf("IDX:%d\t%+v\t%d\n", -1, v2hdr.NumFrames+2, offset)
	//blocks = append(blocks, block{
	//	BlockNum: v2hdr.NumFrames + 2,
	//	BlockOffset: offset,
	//	BlockLen:    blk.FrameNum,
	//})

	fmt.Printf("Blocks: %+v\n", blocks)

	//sparse ranges @todo
	//for (int x = 0; x < header[22]; x++) {
	//	uint32_t st = read3ByteUInt(&header[hoffset]);
	//	uint32_t len = read3ByteUInt(&header[hoffset + 3]);
	//	hoffset += 6;
	//	m_sparseRanges.push_back(std::pair<uint32_t, uint32_t>(st, len));
	//}
	//parseVariableHeaders(header, hoffset);

	switch v2hdr.CompressionType {
	case CompressionType_none:
		fmt.Println("File is not compressed")
		for frameNum := uint32(0); frameNum < v2hdr.NumFrames; frameNum++ {
			fmt.Printf("Frame Number: %d\n", frameNum)

			frameData := make([]byte, v2hdr.NumChannelsPerFrame)
			offset := int64(header.OffsetToChanData)
			offset += int64(v2hdr.NumChannelsPerFrame * frameNum)

			_, err = fp.ReadAt(frameData, offset)
			if err != nil {
				panic(err)
			}
			fmt.Printf("Frame Data: %+v\n", frameData)
		}
		//
		//panic("none compression type not supported")
	case CompressionType_zstd:
		for _, blk := range blocks {
			fmt.Printf("Processing block: %d", blk.BlockNum)
			compressedData := make([]byte, blk.BlockLen)
			_, err = fp.ReadAt(compressedData, int64(blk.BlockOffset))
			if err != nil {
				panic(err)
			}
			data, err := zstd.Decompress(nil, compressedData)
			if err != nil {
				panic(err)
			}
			fmt.Printf("Block: %+v", data)
			return
		}

	case CompressionType_zlib:
		panic("zlib compression type not supported")
	default:
		log.Printf("unknown compression type (%d) not supported", v2hdr.CompressionType)
	}

}
