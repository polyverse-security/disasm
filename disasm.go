package disasm

// #include "disasm.h"
// #cgo CFLAGS: -std=c99
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"unsafe"
)

type Len uint64

type Ptr uintptr

func (p Ptr) String() string {
	str := strconv.FormatUint(uint64(p), 16)
	return "0x" + strings.Repeat("0", 12-len(str)) + str
}

func (p *Ptr) UnmarshalJSON(b []byte) error {
	return errors.New("Unmarshalling not supported for Ptr")
}

func (p Ptr) MarshalJSON() ([]byte, error) {
	return []byte("\"" + p.String() + "\""), nil
}

type Octets []byte

func (o *Octets) UnmarshalJSON(b []byte) error {
	return errors.New("Unmarshalling not supported for octets")
}

func (o Octets) MarshalJSON() ([]byte, error) {
	buffer := &strings.Builder{}
	buffer.WriteString("\"")
	for _, b := range o {
		fmt.Fprintf(buffer, "0x%s", strconv.FormatUint(uint64(b), 16))
	}
	buffer.WriteString("\"")
	return []byte(buffer.String()), nil
}

type iInfo struct {
	info *C.struct_DisAsmInfo
}

type Info struct {
	info   *iInfo
	start  Ptr
	end    Ptr
	length Len
	memory []byte
}

func (info *Info) FindGadgets(instructionsMin int, instructionsMax int, octetsMin int, octetsMax int) ([]*Gadget, []error) {
	gadgets := []*Gadget{}
	errs := []error{}

	for _, gadgetSearchSpec := range gadgetSpecs {
		indices := gadgetSearchSpec.opcode.FindAllIndex(info.memory, -1)
	}

	return gadgets, errs
}

func (info *Info) DecodeGadget(pc Ptr,
	instructionsMin int,
	instructionsMax int,
	octetsMin int,
	octetsMax int) (*Gadget, error) {

	g := &Gadget{
		Address:      pc,
		Instructions: []*Instruction{},
	}

	octetCount := 0


	var nextGadEnd, gadSpec, err := info.FindNextGadgetEnd(pc)

		instruction, err := info.DecodeInstruction(pc0)
		if err != nil {
			return nil, err
		}
		if strings.Contains(instruction.DisAsm, "(bad)") {
			return nil, errors.New("Encountered (bad) instruction")
		} // if

		octetCount += len(instruction.Octets)
		g.Instructions = append(g.Instructions, instruction)

		if (octetCount > octetsMax) || (len(g.Instructions) > instructionsMax) {
			return nil, errors.New("Gadget too long")
		} // if

		pc0 = Ptr(uintptr(pc0) + uintptr(len(instruction.Octets)))

		if validGadEnd {
			if octetCount >= octetsMin && (len(g.Instructions) >= instructionsMin) {
				return g, nil
			} else {
				return nil, errors.New("Gadget too short")
			}
		}

	return nil, errors.New("No gadget found")
} // DecodeGadget()

func (info *Info) DecodeInstruction(pc Ptr) (instruction *Instruction, err error) {
	disAsmInfoPtr := info.info.info

	octetCount := int(C.DisAsmDecodeInstruction(disAsmInfoPtr, C.DisAsmPtr(pc)))
	if octetCount > 0 {
		s := C.GoStringN(&disAsmInfoPtr.disAsmPrintBuffer.data[0], disAsmInfoPtr.disAsmPrintBuffer.index)
		s = strings.TrimRight(s, " ")

		start := pc - info.start
		end := start + Ptr(octetCount)
		octets := info.memory[start:end]
		return &Instruction{Address: pc, Octets: octets, DisAsm: s}, nil
	} // if

	return nil, errors.New("Error with disassembly")
} // DecodeInstruction()

func (info *Info) FindNextGadgetEnd(pc Ptr) (*GadgetEnd) {

	// Start sub-memory at offset to search from
	subMemory := info.memory[pc:]

	var closestGadEnd *gadgetEnd = nil

	for _, gadSpec := range gadgetSpecs {
		gadEndCandidate := gadSpec.opcode.FindIndex(subMemory)
		if gadEndCandidate == nil {
			continue
		}
		if closestGadEnd == nil || closestGadEnd.startOffset > gadEndCandidate[0] {
			closestGadEnd = &gadgetEnd{
				startOffset: gadEndCandidate[0],
				endOffset: gadEndCandidate[1],
			}

			// Shorten sub-memory to next gadget + 10. Reduce search space
			subMemory = info.memory[pc:pc+closestGadEnd.startOffset+maxGadgetLen]
		}
	}
	if closestGadEnd == nil {
		return nil
	}

	return &GadgetEnd{
		StartOffset: Ptr(closestGadEnd.startOffset) + pc,
		EndOffset: Ptr(closestGadEnd.endOffset) + pc,
	}
}

type Instruction struct {
	Address Ptr    `json:"address,string"`
	Octets  Octets `json:"octets"`
	DisAsm  string `json:"disasm"`
}

type Gadget struct {
	Address      Ptr            `json:"address,string"`
	Instructions []*Instruction `json:"instructions"`
}

func (i *Instruction) String() string {

	octetCount := len(i.Octets)
	b := i.Octets
	s := i.Address.String() + " "

	for o := 0; o < 8; o++ {
		if o < octetCount {
			if b[o] < 16 {
				s += "0"
			} // if
			s += strconv.FormatUint(uint64(b[o]), 16)
		} else {
			s += "  "
		} // else
	} // for

	return s + " " + i.DisAsm

	//return i.DisAsm
}

func (g *Gadget) InstructionString() string {
	instrStr := ""
	for _, instr := range g.Instructions {
		if instrStr != "" {
			instrStr = instrStr + ", "
		}
		instrStr = instrStr + instr.DisAsm
	}
	return instrStr
}

func (g *Gadget) String() string {
	sAdr := strconv.FormatUint(uint64(g.Address), 16)
	return "0x" + strings.Repeat("0", 12-len(sAdr)) + sAdr + ": " + g.InstructionString()
}

func InfoInit(s Ptr, e Ptr) Info {
	l := Len(e - s + 1)

	cinfo := C.DisAsmInfoInit(C.DisAsmPtr(s), C.DisAsmPtr(e))
	iinfo := &iInfo{cinfo}
	runtime.SetFinalizer(iinfo, InfoFree)
	info := Info{info: iinfo, start: s, end: e, length: l, memory: C.GoBytes(unsafe.Pointer(s), C.int(l))}

	return info
} // InfoInit()

func InfoInitBytes(s Ptr, e Ptr, b []byte) Info {
	l := Len(e - s + 1)
	if l != Len(len(b)) {
		panic("Disallowed assertion")
	}

	cinfo := C.DisAsmInfoInitBytes(C.DisAsmPtr(s), C.DisAsmPtr(e), unsafe.Pointer(&b[0]))
	iinfo := &iInfo{cinfo}
	runtime.SetFinalizer(iinfo, InfoFree)
	info := Info{info: iinfo, start: s, end: e, length: l, memory: b}

	return info
} // InfoInitBytes()

func InfoFree(i *iInfo) {
	C.DisAsmInfoFree(i.info)
	i.info = nil
} // InfoFree()
