package lookupinfo

// DomainInfo represents the information about a domain.
type DomainInfo struct {
	Source           string   `json:"Source"`
	LookupType       string   `json:"LookupType"`       // LookupType is the type of lookup.
	ViaProxy         bool     `json:"ViaProxy"`         // ViaProxy is the flag to indicate if the lookup is via proxy.
	DomainName       string   `json:"DomainName"`       // DomainName is the name of the domain.
	Registrar        string   `json:"Registrar"`        // Registrar is the registrar of the domain.
	DomainStatus     []string `json:"DomainStatus"`     // DomainStatus is the status of the domain.
	CreationDate     string   `json:"CreationDate"`     // CreationDate is the creation date of the domain.
	ExpiryDate       string   `json:"ExpiryDate"`       // ExpiryDate is the expiry date of the domain.
	NameServer       []string `json:"NameServer"`       // NameServer is the name server of the domain.
	RawResponse      string   `json:"RawResponse"`      // RawResponse is the raw response of the lookup.
	CustomizedResult string   `json:"CustomizedResult"` // CustomizedResult is the customized result of the lookup.
}

type QueryResult struct {
	Status          string   `json:"status"`
	Source          string   `json:"source"`
	QueriedAt       string   `json:"queriedAt"`
	Order           int      `json:"order"`
	Domain          string   `json:"domain"`
	LookupType      string   `json:"lookupType"`
	ViaProxy        bool     `json:"viaProxy"`
	QueryError      string   `json:"queryError"`
	RegisterStatus  string   `json:"registerStatus"`
	CreatedDate     string   `json:"createdDate"`
	ExpiryDate      string   `json:"expiryDate"`
	NameServer      []string `json:"nameServer"`
	DnsLite         string   `json:"dnsLite"`
	RawDomainStatus []string `json:"rawDomainStatus"`
	DomainStatus    string   `json:"domainStatus"`
	RawResponse     string   `json:"rawResponse"`
}

type QueryCsvResult struct {
	Source          string `csv:"Source"`
	QueriedAt       string `csv:"Queried At"`
	Domain          string `csv:"Domain"`
	LookupType      string `csv:"Lookup Type"`
	ViaProxy        string `csv:"Via Proxy,omitempty"`
	Status          string `csv:"Status"`
	ErrorInfo       string `csv:"Error Info,omitempty"`
	CreatedDate     string `csv:"Created Date,omitempty"`
	ExpiryDate      string `csv:"Expiry Date,omitempty"`
	NameServer      string `csv:"Name Server,omitempty"`
	DnsLite         string `csv:"Dns Lite,omitempty"`
	RawDomainStatus string `csv:"Raw Domain Status,omitempty"`
	DomainStatus    string `csv:"Domain Status,omitempty"`
}
