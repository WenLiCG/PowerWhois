package register

// RegisterInfo represents the information about a register.
type RegisterInfo struct {
	AuditID        string `json:"auditId,omitempty"`
	RegisterType   string `json:"registerType"`   // RegisterType is the type of register.
	DomainName     string `json:"domainName"`     // DomainName is the name of the domain.
	RegisterStatus string `json:"registerStatus"` // RegisterStatus is the status of the register result.
	RawResponse    string `json:"rawResponse"`    // RawResponse is the raw response of the register API.
}
