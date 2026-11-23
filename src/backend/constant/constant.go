package constant

const (
	// WhoisQuery is the type for querying whois information without using a proxy.
	WhoisQuery = "whoisQuery"

	// WhoisQueryWithProxy is the type for querying whois information with using a proxy.
	WhoisQueryWithProxy = "whoisQueryWithProxy"

	// DnsQuery is the type for querying DNS information.
	DnsQuery = "dnsQuery"

	// MixedQuery is the type for querying whois and dns information.
	MixedQuery = "mixedQuery"
)

const (
	// Redis key for bulk check query type
	BulkCheckQueryTypeRedisKey = "bulkCheckQueryType"

	// Redis key for bulk check raw domains
	BulkCheckRawDomainsRedisKey = "bulkCheckRawDomains"

	// Redis key for bulk check unique domains
	BulkCheckUniqueDomainsRedisKey = "bulkCheckUniqueDomains"

	// Redis key for bulk check unique domains count
	BulkCheckUniqueDomainsCountRedisKey = "bulkCheckUniqueDomainsCount"

	// Redis key for bulk check taken result
	BulkCheckTakenResultRedisKey = "bulkCheckTakenResult"

	// Redis key for bulk check free result
	BulkCheckFreeResultRedisKey = "bulkCheckFreeResult"

	// Redis key for bulk check error result
	BulkCheckErrorResultRedisKey = "bulkCheckErrorResult"

	// Redis key for bulk check status
	BulkCheckStatusRedisKey = "bulkCheckStatus"
)

const (
	// BulkCheckStatusIdle indicates that the bulk check is not running.
	BulkCheckStatusIdle = "idle"

	// BulkCheckStatusInit indicates that the bulk check is initializing.
	BulkCheckStatusInit = "init"

	// BulkCheckStatusUniquing indicates that the bulk check is processing the raw domains to unique domains.
	BulkCheckStatusUniquing = "uniquing"

	// BulkCheckStatusRunning indicates that the bulk check is running.
	BulkCheckStatusRunning = "running"

	// BulkCheckStatusPaused indicates that the bulk check is paused.
	BulkCheckStatusPaused = "paused"

	// BulkCheckStatusDone indicates that the bulk check is finished.
	BulkCheckStatusDone = "done"

	// BulkCheckStatusCanceled indicates that the bulk check is canceled.
	BulkCheckStatusCanceled = "canceled"

	// BulkCheckStatusError indicates that the bulk check is in an error state.
	BulkCheckStatusError = "error"
)

const (
	// WebsocketRequestEventPing is the event name for a ping message.
	WebsocketRequestEventPing = "ping"

	// WebsocketRequestEventAdminAuth is the event name for an admin auth message.
	WebsocketRequestEventAdminAuth = "adminAuth"

	// WebsocketRequestEventBulkCheckStart is the event name for starting a bulk check.
	WebsocketRequestEventBulkCheckStart = "bulkCheckStart"

	// WebsocketRequestEventBulkCheckPause is the event name for pausing a bulk check.
	WebsocketRequestEventBulkCheckPause = "bulkCheckPause"

	// WebsocketRequestEventBulkCheckResume is the event name for resuming a bulk check.
	WebsocketRequestEventBulkCheckResume = "bulkCheckResume"

	// WebsocketRequestEventBulkCheckCancel is the event name for canceling a bulk check.
	WebsocketRequestEventBulkCheckCancel = "bulkCheckCancel"

	// WebsocketRequestEventBulkCheckClear is the event name for clearing a bulk check.
	WebsocketRequestEventBulkCheckClear = "bulkCheckClear"

	// WebsocketRequestEventBulkRecheckErrorDomains is the event name for requerying the error domains of a bulk check.
	WebsocketRequestEventBulkRecheckErrorDomains = "bulkRecheckErrorDomains"

	// WebsocketRequestEventWebCheck is the event name for a web check message.
	WebsocketRequestEventWebCheck = "webCheck"

	// WebsocketRequestEventTypoCheck is the event name for a typo check message.
	WebsocketRequestEventTypoCheck = "typoCheck"

	// WebsocketRequestEventRegister is the event name for a register message.
	WebsocketRequestEventRegister = "register"
)

const (
	// WebsocketResponseEventPong is the event name for a pong response.
	WebsocketResponseEventPong = "pong"

	// WebsocketResponseEventWebCheckDomains is the event name for a list of web check domains.
	WebsocketResponseEventWebCheckDomains = "webCheckDomains"

	// WebsocketResponseEventWebCheckResult is the event name for a web check result.
	WebsocketResponseEventWebCheckResult = "webCheckResult"

	// WebsocketResponseEventTypoResult is the event name for a typo check result.
	WebsocketResponseEventTypoResult = "typoResult"

	// WebsocketResponseEventRegisterResult is the event name for a register result.
	WebsocketResponseEventRegisterResult = "registerResult"
)

const (
	// WebsocketResponseBulkCheckErrorEvent is the event name for a bulk check error.
	WebsocketResponseBulkCheckErrorEvent = "bulkCheckError"

	// WebsocketResponseBulkCheckInfoEvent is the event name for the bulk check info.
	WebsocketResponseBulkCheckInfoEvent = "bulkCheckInfo"

	// WebsocketResponseWebCheckErrorEvent is the event name for a web check error.
	WebsocketResponseWebCheckErrorEvent = "webCheckError"

	// WebsocketResponseTypoCheckErrorEvent is the event name for a typo check error.
	WebsocketResponseTypoCheckErrorEvent = "typoCheckError"

	// WebsocketResponseRegisterErrorEvent is the event name for a register error.
	WebsocketResponseRegisterErrorEvent = "registerError"
)

const (
	// DomainRegisterStatusTaken is the status when a domain is taken.
	DomainRegisterStatusTaken = "Taken"

	// DomainRegisterStatusFree is the status when a domain is free.
	DomainRegisterStatusFree = "Free"

	// DomainRegisterStatusError is the status when a domain query error occurs.
	DomainRegisterStatusError = "Error"
)

const (
	// DomainStatusActive is the status when a domain is active.
	DomainStatusActive = "Active"

	// DomainStatusExpired is the status when a domain is expired.
	DomainStatusExpired = "Expired"

	// DomainStatusRedemptionPeriod is the status when a domain is in the redemption period.
	DomainStatusRedemptionPeriod = "RedemptionPeriod"

	// DomainStatusPendingDelete is the status when a domain is pending deletion.
	DomainStatusPendingDelete = "PendingDelete"

	// DomainStatusUnknown is the status when the domain status is unknown.
	DomainStatusUnknown = "Unknown"
)

const (
	// LookupTypeWhois is the type for a whois lookup.
	LookupTypeWhois = "whois"

	// LookupTypeRDAP is the type for a rdap lookup.
	LookupTypeRDAP = "rdap"

	// LookupTypeDNS is the type for a dns lookup.
	LookupTypeDNS = "dns"
)

const (
	// TypoTypeWww is the type for a www typo.
	TypoTypeWww = "www"

	// TypoTypeSkipLetter is the type for a skip letter typo.
	TypoTypeSkipLetter = "skipLetter"

	// TypoTypeDoubleLetter is the type for a double letter typo.
	TypoTypeDoubleLetter = "doubleLetter"

	// TypoTypeReverseLetter is the type for a reverse letter typo.
	TypoTypeReverseLetter = "reverseLetter"

	// TypoTypeInsertedLetter is the type for a inserted letter typo.
	TypoTypeInsertedLetter = "insertedLetter"

	// TypoTypeWrongHorizontalKey is the type for a wrong horizontal key typo.
	TypoTypeWrongHorizontalKey = "wrongHorizontalKey"

	// TypoTypeWrongVerticalKey is the type for a wrong vertical key typo.
	TypoTypeWrongVerticalKey = "wrongVerticalKey"

	// TypoTypeWrongTlds is the type for a wrong tlds typo.
	TypoTypeWrongTlds = "wrongTlds"

	// TypoTypeCustomizedReplace is the type for a customized replace typo.
	TypoTypeCustomizedReplace = "customizedReplace"
)

const (
	// RegisterStatusSuccess is the status when a register is successful.
	RegisterStatusSuccess = "success"

	// RegisterStatusFailed is the status when a register is failed.
	RegisterStatusFailed = "failed"

	// RegisterStatusError is the status when a register is error.
	RegisterStatusError = "error"
)
