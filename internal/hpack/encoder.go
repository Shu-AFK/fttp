package hpack

const DefaultMaxDynamicTableSize = 4096

type Encoder struct {
	Table               *IndexAddressSpace
	MaxDynamicTableSize int
	NextIndex           int
}

func NewEncoder(dynamicTableSize ...int) *Encoder {
	staticTable := *initIndexAddressSpace()

	maxTableSize := DefaultMaxDynamicTableSize
	if len(dynamicTableSize) > 0 {
		maxTableSize = dynamicTableSize[0]
	}

	dynamicTable := make(IndexAddressSpace, maxTableSize)
	table := append(staticTable, dynamicTable...)

	return &Encoder{
		Table:               &table,
		MaxDynamicTableSize: maxTableSize,
		NextIndex:           STATIC_TABLE_SIZE + 1,
	}
}
