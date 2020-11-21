package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/DataDog/zstd"
	"log"
	"os"
)

const (
	V2FSEQ_HEADER_SIZE = 32 //copied from https://github.com/smeighan/xLights/blob/15f42b39a38861564518c353b9e3f24ee790de05/xLights/FSEQFile.cpp#L710
)

//type CompressionType uint8
const (
	CompressionType_none = 0
	CompressionType_zstd = 1
	CompressionType_zlib = 2
)

// FSEQ file format: https://github.com/FalconChristmas/fpp/blob/master/docs/FSEQ_Sequence_File_Format.txt
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

//numberOfBlocks (8bytes) - compress block index

// this is used to parse fseq blocks once uncompressed
type fseqv2_block struct {
	//0-3 - frame number
	FrameNum uint32
	//4-7 - length of block
	BlockLen uint32
}

// It seems like sparse ranges may be created as a part of the FPP Connect utility

//numberOfSparseRanges (6bytes) - sparse range definitions
type fseqv2_sparse struct {
	//0-2 - start channel number
	StartChan [3]byte
	//3-5 - number of channels
	NumChan [3]byte
}

// When parsing the blocks, we will calculate the offset and store it
// a block is a group of frames
type block struct {
	StartFrameNum      uint32
	BlockOffset        uint32
	BlockCompressedLen uint32
}

func printStructJson(prefix string, v interface{})  {
	jsonStr, err := json.MarshalIndent(v, "", "    ")
	if err != nil {
		log.Fatalf(err.Error())
	}
	fmt.Printf("%s %s\n", prefix, jsonStr)
}

type Frame struct {
	FrameNum uint32
	Data []byte
}

func main() {
	//flag.String("file", )

	//flag.Parse()
	//
	//fileName := flag.Arg(0)

	//fileName := "samples/xlights/test1.fseq"
	//fileName := "samples/xlights/test2-3000.fseq"

	//fileName := "Carol of the Bells - Trans-Siberian Orchestra v2.fseq"
	fileName := "samples/Carol of the Bells - Trans-Siberian Orchestra v2 master.fseq"
	//fileName := "Carol of the Bells - Trans-Siberian Orchestra v2 renard01.fseq"
	//fileName := "Carol of the Bells - Trans-Siberian Orchestra v2 pixels.fseq"


	fmt.Printf("\n\nProcessing File: %s\n", fileName)

	if len(fileName) <= 0 {
		panic("usage")
	}

	fp, err := os.Open(fileName)
	if err != nil {
		panic(err)
	}
	defer fp.Close()

	header := fseqHeader{}

	err = binary.Read(fp, binary.LittleEndian, &header)
	if err != nil { //@todo this will error if the file doesn't match right?
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

	printStructJson("Header:", header)
	//fmt.Printf("HEADER: %+v\n", header)

	v2hdr := fseqv2Header{}

	err = binary.Read(fp, binary.LittleEndian, &v2hdr)
	if err != nil {
		panic(err)
	}

	printStructJson("Header v2:", v2hdr)
	//fmt.Printf("HEADER v2: %+v\n", v2hdr)

	//if v2hdr.NumSparseRanges > 0 {
	//	panic("sparse ranges are unsupported")
	//}

	// move the pointer to the end of the header
	_, err = fp.Seek(int64(V2FSEQ_HEADER_SIZE), 0)
	if err != nil {
		panic(err)
	}

	// This is a slice of the blocks we have decoded in the file
	blocks := []block{}

	offset := uint32(header.OffsetToChanData)
	for blockNum := uint8(0); blockNum < v2hdr.NumCompressedBlocks; blockNum++ {
		blk := fseqv2_block{}
		err = binary.Read(fp, binary.LittleEndian, &blk)
		if err != nil {
			panic(err)
		}

		if blk.BlockLen > 0 {
			//fmt.Printf("IDX:%d \t %+v \t Start: %d \t End: %d\n", blockNum, blk, offset, offset+blk.BlockCompressedLen)
			blocks = append(blocks, block{
				StartFrameNum:      blk.FrameNum,
				BlockOffset:        offset,
				BlockCompressedLen: blk.BlockLen,
			})
			offset += blk.BlockLen
		} else {
			//block with zero length data
		}
	}
	//                                             why is this +2 ?
	//fmt.Printf("IDX:%d\t%+v\t%d\n", -1, v2hdr.NumFrames+2, offset)
	//blocks = append(blocks, block{
	//	StartFrameNum: v2hdr.NumFrames + 2,
	//	BlockOffset: offset,
	//	BlockCompressedLen:    blk.FrameNum,
	//})

	//fmt.Printf("Blocks: %+v\n", blocks)
	printStructJson("Blocks:", blocks)

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
			fmt.Printf("Decompressing block: %d\n", blk.StartFrameNum)
			compressedData := make([]byte, blk.BlockCompressedLen)
			_, err = fp.ReadAt(compressedData, int64(blk.BlockOffset))
			if err != nil {
				panic(err)
			}
			blockValues, err := zstd.Decompress(nil, compressedData)
			if err != nil {
				panic(err)
			}


			numFramesInBlock := uint32(len(blockValues)) / v2hdr.NumChannelsPerFrame

			fmt.Printf("TotalChan: %d NumFramesInBlk: %d\n", len(blockValues), numFramesInBlock)

			// for each frame in the block
			//for frameIdx := uint32(0); frameIdx < numFramesInBlock; frameIdx ++ {
			//	//fmt.Printf("low: %d\thigh: %d\n", frameIdx*v2hdr.NumChannelsPerFrame, frameIdx*v2hdr.NumChannelsPerFrame+v2hdr.NumChannelsPerFrame-1)
			//	low := frameIdx*v2hdr.NumChannelsPerFrame
			//	high :=frameIdx*v2hdr.NumChannelsPerFrame+v2hdr.NumChannelsPerFrame-1
			//	newFrame := Frame{
			//		FrameNum: frameIdx+1,
			//		Data:     blockValues[low:high],
			//	}
			//	fmt.Printf("Frame: %+v\n", newFrame)
			//	//printStructJson("Frame:", newFrame)
			//}

			//for a := range len(blockValues) / v2hdr.NumChannelsPerFrame
			//{
			//
			//}
			//for chanIdx, value := range data {
			//	newFrame := Frame{
			//		FrameNum: uint32(chanIdx) % v2hdr.NumChannelsPerFrame,
			//		Data:     nil,
			//	}
			//	fmt.Printf("chan: %d val: %d\n", , value)
			//
			//}
			//fmt.Printf("Block: %+v", data)
			//return
		}

	case CompressionType_zlib:
		panic("zlib compression type not supported")
	default:
		log.Printf("unknown compression type (%d) not supported", v2hdr.CompressionType)
	}

}
