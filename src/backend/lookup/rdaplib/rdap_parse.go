package rdaplib

import (
	"fmt"
	"strings"

	"typonamer/constant"
	"typonamer/lookup/lookupinfo"
	"typonamer/utils"

	"github.com/dromara/carbon/v2"
	"github.com/duke-git/lancet/v2/convertor"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/duke-git/lancet/v2/strutil"
	"github.com/openrdap/rdap"
)

// ParseRDAPResponseforDomain function is used to parse the RDAP response for a given domain.
func ParseRDAPResponseforDomain(response *rdap.Domain) lookupinfo.DomainInfo {
	// Initialize the DomainInfo struct
	domainInfo := lookupinfo.DomainInfo{
		LookupType:   constant.LookupTypeRDAP,
		DomainName:   response.LDHName,
		Registrar:    getRegistrar(response.Entities),
		DomainStatus: response.Status,
		NameServer:   getNameServer(response.Nameservers),
	}

	// Get the creation and expiry date from the response events
	creationDate, expiryDate := getDomainDates(response.Events)
	domainInfo.CreationDate = creationDate
	domainInfo.ExpiryDate = expiryDate

	// Return the parsed DomainInfo
	return domainInfo
}

// getRegistrar is a function that takes the response entities and returns the registrar name.
// It will loop through the entities and check if the entity has the role of "registrar".
// If it does, it will check if the entity has a vcard property and if the vcard property has a "fn" property.
// If it does, it will return the value of the "fn" property. If not, it will return the handle of the entity.
func getRegistrar(responseEntities []rdap.Entity) string {
	// Loop through the entities and check if the entity has the role of "registrar"
	for _, entity := range responseEntities {
		// Convert the roles to lowercase
		roles := slice.Map(entity.Roles, utils.LowerString)

		// Check if the entity has the role of "registrar"
		if slice.Contain(roles, "registrar") {
			// Check if the entity has a vcard property
			if entity.VCard != nil {
				// Loop through the vcard properties and check if the property name is "fn"
				for _, property := range entity.VCard.Properties {
					if property.Name == "fn" {
						// Return the value of the "fn" property
						return convertor.ToString(property.Value)
					}
				}
			} else {
				// Return the handle of the entity
				return entity.Handle
			}
		}
	}

	// Return an empty string if the registrar is not found
	return ""
}

// getDomainDates takes the response events and returns the creation and expiry dates of the domain.
// It loops through the events and checks if the event action is "registration" or "expiration".
// If the event action is "registration", it sets the creation date to the date of the event.
// If the event action is "expiration", it sets the expiry date to the date of the event.
// The dates are parsed using the Carbon library and set to UTC timezone.
// The function returns the creation and expiry dates as a tuple.
func getDomainDates(responseEvents []rdap.Event) (string, string) {
	var creationDate, expiryDate string
	for _, event := range responseEvents {
		switch strings.ToLower(event.Action) {
		case "registration":
			// Set the creation date to the date of the event
			creationDate = carbon.SetTimezone(carbon.UTC).Parse(event.Date).ToDateTimeString()
		case "expiration":
			// Set the expiry date to the date of the event
			expiryDate = carbon.SetTimezone(carbon.UTC).Parse(event.Date).ToDateTimeString()
		}
	}
	return creationDate, expiryDate
}

// getNameServer takes the response nameservers and returns the nameservers as a slice of strings.
// The function loops through the nameservers and appends the LDHName of each nameserver to the slice.
// The nameservers are converted to lowercase before being appended to the slice.
// The function returns the slice of nameservers.
func getNameServer(responseNameservers []rdap.Nameserver) []string {
	nameServers := []string{}

	for _, ns := range responseNameservers {
		nameServers = append(nameServers, strutil.Trim(ns.LDHName, "."))
	}

	return slice.Map(nameServers, utils.LowerString)
}

// getWhoisStyleRawResponse takes the response and returns the raw response in whois style.
// It loops through the http responses and appends the body of each http response to the raw response.
// The function returns the raw response in whois style.
func getWhoisStyleRawResponse(response *rdap.Response) string {
	whoisStyleResponse := response.ToWhoisStyleResponse()
	rawResponse := ""
	for key, value := range whoisStyleResponse.Data {
		for _, line := range value {
			rawResponse += fmt.Sprintf("%s: %s\n", key, line)
		}
	}
	return rawResponse
}
