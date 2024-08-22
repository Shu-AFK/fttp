package hpack

type HeaderField struct {
	HeaderFieldName  string
	HeaderFieldValue string
	NeverIndexed     bool
}

type IndexAddressSpace map[uint]*HeaderField

const STATIC_TABLE_SIZE = 61

func NewHeaderField(name string, value string, neverIndexed bool) *HeaderField {
	return &HeaderField{
		HeaderFieldName:  name,
		HeaderFieldValue: value,
	}
}

func initIndexAddressSpace() *IndexAddressSpace {
	IndexAddressSpace_ := make(IndexAddressSpace, STATIC_TABLE_SIZE)
	IndexAddressSpace_[1] = NewHeaderField(":authority", "", false)
	IndexAddressSpace_[2] = NewHeaderField(":method", "GET", false)
	IndexAddressSpace_[3] = NewHeaderField(":method", "POST", false)
	IndexAddressSpace_[4] = NewHeaderField(":path", "/", false)
	IndexAddressSpace_[5] = NewHeaderField(":path", "/index.html", false)
	IndexAddressSpace_[6] = NewHeaderField(":scheme", "http", false)
	IndexAddressSpace_[7] = NewHeaderField(":scheme", "https", false)
	IndexAddressSpace_[8] = NewHeaderField(":status", "200", false)
	IndexAddressSpace_[9] = NewHeaderField(":status", "204", false)
	IndexAddressSpace_[10] = NewHeaderField(":status", "206", false)
	IndexAddressSpace_[11] = NewHeaderField(":status", "304", false)
	IndexAddressSpace_[12] = NewHeaderField(":status", "400", false)
	IndexAddressSpace_[13] = NewHeaderField(":status", "404", false)
	IndexAddressSpace_[14] = NewHeaderField(":status", "500", false)
	IndexAddressSpace_[15] = NewHeaderField("accept-charset", "", false)
	IndexAddressSpace_[16] = NewHeaderField("accept-encoding", "gzip, deflate", false)
	IndexAddressSpace_[17] = NewHeaderField("accept-language", "", false)
	IndexAddressSpace_[18] = NewHeaderField("accept-ranges", "", false)
	IndexAddressSpace_[19] = NewHeaderField("accept", "", false)
	IndexAddressSpace_[20] = NewHeaderField("access-control-allow-origin", "", false)
	IndexAddressSpace_[21] = NewHeaderField("age", "", false)
	IndexAddressSpace_[22] = NewHeaderField("allow", "", false)
	IndexAddressSpace_[23] = NewHeaderField("authorization", "", false)
	IndexAddressSpace_[24] = NewHeaderField("cache-control", "", false)
	IndexAddressSpace_[25] = NewHeaderField("content-disposition", "", false)
	IndexAddressSpace_[26] = NewHeaderField("content-encoding", "", false)
	IndexAddressSpace_[27] = NewHeaderField("content-language", "", false)
	IndexAddressSpace_[28] = NewHeaderField("content-length", "", false)
	IndexAddressSpace_[29] = NewHeaderField("content-location", "", false)
	IndexAddressSpace_[30] = NewHeaderField("content-range", "", false)
	IndexAddressSpace_[31] = NewHeaderField("content-type", "", false)
	IndexAddressSpace_[32] = NewHeaderField("cookie", "", false)
	IndexAddressSpace_[33] = NewHeaderField("date", "", false)
	IndexAddressSpace_[34] = NewHeaderField("etag", "", false)
	IndexAddressSpace_[35] = NewHeaderField("expect", "", false)
	IndexAddressSpace_[36] = NewHeaderField("expires", "", false)
	IndexAddressSpace_[37] = NewHeaderField("from", "", false)
	IndexAddressSpace_[38] = NewHeaderField("host", "", false)
	IndexAddressSpace_[39] = NewHeaderField("if-match", "", false)
	IndexAddressSpace_[40] = NewHeaderField("if-modified-since", "", false)
	IndexAddressSpace_[41] = NewHeaderField("if-none-match", "", false)
	IndexAddressSpace_[42] = NewHeaderField("if-range", "", false)
	IndexAddressSpace_[43] = NewHeaderField("if-unmodified-since", "", false)
	IndexAddressSpace_[44] = NewHeaderField("last-modified", "", false)
	IndexAddressSpace_[45] = NewHeaderField("link", "", false)
	IndexAddressSpace_[46] = NewHeaderField("location", "", false)
	IndexAddressSpace_[47] = NewHeaderField("max-forwards", "", false)
	IndexAddressSpace_[48] = NewHeaderField("proxy-authenticate", "", false)
	IndexAddressSpace_[49] = NewHeaderField("proxy-authorization", "", false)
	IndexAddressSpace_[50] = NewHeaderField("range", "", false)
	IndexAddressSpace_[51] = NewHeaderField("referer", "", false)
	IndexAddressSpace_[52] = NewHeaderField("refresh", "", false)
	IndexAddressSpace_[53] = NewHeaderField("retry-after", "", false)
	IndexAddressSpace_[54] = NewHeaderField("server", "", false)
	IndexAddressSpace_[55] = NewHeaderField("set-cookie", "", false)
	IndexAddressSpace_[56] = NewHeaderField("strict-transport-security", "", false)
	IndexAddressSpace_[57] = NewHeaderField("transfer-encoding", "", false)
	IndexAddressSpace_[58] = NewHeaderField("user-agent", "", false)
	IndexAddressSpace_[59] = NewHeaderField("vary", "", false)
	IndexAddressSpace_[60] = NewHeaderField("via", "", false)
	IndexAddressSpace_[61] = NewHeaderField("www-authenticate", "", false)

	return &IndexAddressSpace_
}

func mergeTables(m1, m2 IndexAddressSpace) *IndexAddressSpace {
	merged := make(IndexAddressSpace)
	for k, v := range m1 {
		merged[k] = v
	}
	for k, v := range m2 {
		merged[k] = v
	}
	return &merged
}
