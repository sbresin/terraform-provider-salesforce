package metadata

import (
	"encoding/xml"
	"time"
)

// XML namespaces for SOAP Metadata API
const (
	nsSOAPEnv  = "http://schemas.xmlsoap.org/soap/envelope/"
	nsMetadata = "http://soap.sforce.com/2006/04/metadata"
)

// Envelope is the SOAP envelope wrapper for requests.
type Envelope struct {
	XMLName xml.Name `xml:"soapenv:Envelope"`
	XMLNS   string   `xml:"xmlns:soapenv,attr"`
	XMLNSm  string   `xml:"xmlns:met,attr"`
	Header  Header   `xml:"soapenv:Header"`
	Body    Body     `xml:"soapenv:Body"`
}

// Header contains the SOAP header with session information.
type Header struct {
	SessionHeader SessionHeader `xml:"met:SessionHeader"`
}

// SessionHeader contains the session ID for authentication.
type SessionHeader struct {
	SessionID string `xml:"met:sessionId"`
}

// Body is the SOAP body containing the actual request/response.
type Body struct {
	Content any `xml:",any"`
}

// ResponseEnvelope is the SOAP envelope wrapper for responses.
type ResponseEnvelope struct {
	XMLName xml.Name     `xml:"Envelope"`
	Body    ResponseBody `xml:"Body"`
}

// ResponseBody holds the response content or fault.
type ResponseBody struct {
	Fault   *Fault `xml:"Fault,omitempty"`
	Content []byte `xml:",innerxml"`
}

// Fault represents a SOAP fault response.
type Fault struct {
	FaultCode   string `xml:"faultcode"`
	FaultString string `xml:"faultstring"`
}

// --- Request types ---

// ReadMetadataRequest is the request body for readMetadata.
type ReadMetadataRequest struct {
	XMLName   xml.Name `xml:"met:readMetadata"`
	Type      string   `xml:"met:type"`
	FullNames []string `xml:"met:fullNames"`
}

// ListMetadataRequest is the request body for listMetadata.
type ListMetadataRequest struct {
	XMLName     xml.Name            `xml:"met:listMetadata"`
	Queries     []ListMetadataQuery `xml:"met:queries"`
	AsOfVersion string              `xml:"met:asOfVersion,omitempty"`
}

// ListMetadataQuery specifies which metadata type to list.
type ListMetadataQuery struct {
	Type   string `xml:"met:type"`
	Folder string `xml:"met:folder,omitempty"`
}

// DeployRequest is the request body for deploy.
type DeployRequest struct {
	XMLName       xml.Name      `xml:"met:deploy"`
	ZipFile       string        `xml:"met:ZipFile"` // base64-encoded zip
	DeployOptions DeployOptions `xml:"met:DeployOptions"`
}

// DeployOptions controls deployment behavior.
type DeployOptions struct {
	SinglePackage   bool `xml:"met:singlePackage"`
	RollbackOnError bool `xml:"met:rollbackOnError"`
	CheckOnly       bool `xml:"met:checkOnly,omitempty"`
}

// CheckDeployStatusRequest is the request body for checkDeployStatus.
type CheckDeployStatusRequest struct {
	XMLName        xml.Name `xml:"met:checkDeployStatus"`
	AsyncProcessID string   `xml:"met:asyncProcessId"`
	IncludeDetails bool     `xml:"met:includeDetails"`
}

// --- Response types ---

// ReadMetadataResponse wraps the readMetadata response.
type ReadMetadataResponse struct {
	XMLName xml.Name `xml:"readMetadataResponse"`
	Result  struct {
		Records []CertificateRecord `xml:"records"`
	} `xml:"result"`
}

// CertificateRecord is the raw certificate data from the API.
type CertificateRecord struct {
	FullName                        string `xml:"fullName"`
	MasterLabel                     string `xml:"masterLabel"`
	Certificate                     string `xml:"content"` // base64-encoded PEM (Salesforce returns this as "content")
	CaSigned                        bool   `xml:"caSigned,omitempty"`
	EncryptedWithPlatformEncryption bool   `xml:"encryptedWithPlatformEncryption,omitempty"`
	ExpirationDate                  string `xml:"expirationDate,omitempty"`
	KeySize                         int    `xml:"keySize,omitempty"`
	PrivateKeyExportable            bool   `xml:"privateKeyExportable,omitempty"`
}

type readCustomMetadataResponse struct {
	XMLName xml.Name            `xml:"readMetadataResponse"`
	Records []customMetadataRaw `xml:"result>records"`
}

type customMetadataRaw struct {
	FullName  string `xml:"fullName"`
	Label     string `xml:"label"`
	Protected bool   `xml:"protected"`
	Values    []struct {
		Field string `xml:"field"`
		Value struct {
			Type string `xml:"type,attr"`
			Nil  bool   `xml:"nil,attr"` // xsi:nil; the envelope's xmlns:xsi is lost in Body.Content
			Text string `xml:",chardata"`
		} `xml:"value"`
	} `xml:"values"`
}

type readCustomObjectResponse struct {
	XMLName xml.Name `xml:"readMetadataResponse"`
	Records []struct {
		FullName string `xml:"fullName"`
		Fields   []struct {
			FullName string `xml:"fullName"`
			Type     string `xml:"type"`
		} `xml:"fields"`
	} `xml:"result>records"`
}

// ListMetadataResponse wraps the listMetadata response.
type ListMetadataResponse struct {
	XMLName xml.Name         `xml:"listMetadataResponse"`
	Result  []FileProperties `xml:"result"`
}

// FileProperties describes a metadata component.
type FileProperties struct {
	FullName string `xml:"fullName"`
	Type     string `xml:"type"`
}

// DeployResponse wraps the deploy response.
type DeployResponse struct {
	XMLName xml.Name    `xml:"deployResponse"`
	Result  AsyncResult `xml:"result"`
}

// AsyncResult is returned when starting an async operation.
type AsyncResult struct {
	ID    string `xml:"id"`
	Done  bool   `xml:"done"`
	State string `xml:"state"`
}

// CheckDeployStatusResponse wraps the checkDeployStatus response.
type CheckDeployStatusResponse struct {
	XMLName xml.Name     `xml:"checkDeployStatusResponse"`
	Result  DeployResult `xml:"result"`
}

// DeployResult contains the status of a deployment.
type DeployResult struct {
	ID                       string             `xml:"id"`
	Done                     bool               `xml:"done"`
	Success                  bool               `xml:"success"`
	Status                   string             `xml:"status"`
	ErrorMessage             string             `xml:"errorMessage,omitempty"`
	ErrorStatusCode          string             `xml:"errorStatusCode,omitempty"`
	ComponentFailures        []ComponentFailure `xml:"details>componentFailures,omitempty"`
	NumberComponentsTotal    int                `xml:"numberComponentsTotal,omitempty"`
	NumberComponentsDeployed int                `xml:"numberComponentsDeployed,omitempty"`
	NumberComponentErrors    int                `xml:"numberComponentErrors,omitempty"`
}

// ComponentFailure describes a failed component in a deployment.
type ComponentFailure struct {
	FullName      string `xml:"fullName"`
	FileName      string `xml:"fileName,omitempty"`
	Problem       string `xml:"problem"`
	ProblemType   string `xml:"problemType"`
	ComponentType string `xml:"componentType"`
	LineNumber    int    `xml:"lineNumber,omitempty"`
	ColumnNumber  int    `xml:"columnNumber,omitempty"`
}

// --- Public types ---

type CertificateCreate struct {
	FullName             string
	MasterLabel          string
	KeySize              int
	PrivateKeyExportable bool
}

// CustomMetadataDeploy describes a Custom Metadata record to create or update.
type CustomMetadataDeploy struct {
	Type          string // CMDT API name without __mdt
	DeveloperName string
	Label         string
	Protected     bool
	Values        map[string]string // field API name -> value; every other field is cleared
}

// CustomMetadataRecord is a Custom Metadata record as read from the org.
type CustomMetadataRecord struct {
	FullName  string
	Label     string
	Protected bool
	Values    map[string]FieldValue // non-null fields only
}

// FieldValue is a Custom Metadata field value with its xsi:type (e.g. "xsd:double").
type FieldValue struct {
	Type  string
	Value string
}

// Certificate represents a Salesforce certificate with all metadata.
type Certificate struct {
	FullName                        string
	MasterLabel                     string
	Certificate                     string // base64-encoded PEM certificate chain
	CaSigned                        bool
	EncryptedWithPlatformEncryption bool
	ExpirationDate                  *time.Time
	KeySize                         int
	PrivateKeyExportable            bool
}
