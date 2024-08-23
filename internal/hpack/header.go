package hpack

type HeaderField struct {
	HeaderFieldName  string
	HeaderFieldValue string
	NeverIndexed     bool
}

type IndexAddressSpace []HeaderField

const STATIC_TABLE_SIZE = 61

func NewHeaderField(name string, value string, neverIndexed bool) *HeaderField {
	return &HeaderField{
		HeaderFieldName:  name,
		HeaderFieldValue: value,
	}
}

func initIndexAddressSpace() *IndexAddressSpace {
	IndexAddressSpace_ := make(IndexAddressSpace, STATIC_TABLE_SIZE)
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField(":authority", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField(":method", "GET", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField(":method", "POST", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField(":path", "/", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField(":path", "/index.html", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField(":scheme", "http", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField(":scheme", "https", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField(":status", "200", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField(":status", "204", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField(":status", "206", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField(":status", "304", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField(":status", "400", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField(":status", "404", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField(":status", "500", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("accept-charset", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("accept-encoding", "gzip, deflate", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("accept-language", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("accept-ranges", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("accept", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("access-control-allow-origin", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("age", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("allow", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("authorization", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("cache-control", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("content-disposition", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("content-encoding", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("content-language", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("content-length", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("content-location", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("content-range", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("content-type", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("cookie", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("date", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("etag", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("expect", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("expires", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("from", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("host", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("if-match", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("if-modified-since", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("if-none-match", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("if-range", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("if-unmodified-since", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("last-modified", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("link", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("location", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("max-forwards", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("proxy-authenticate", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("proxy-authorization", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("range", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("referer", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("refresh", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("retry-after", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("server", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("set-cookie", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("strict-transport-security", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("transfer-encoding", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("user-agent", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("vary", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("via", "", false))
	IndexAddressSpace_ = append(IndexAddressSpace_, *NewHeaderField("www-authenticate", "", false))

	return &IndexAddressSpace_
}
