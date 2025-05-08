package hermine

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_diDocument_createComment(t *testing.T) {
	tests := []struct {
		name           string
		documentFields map[string]diDocumentField
		items          *[]diDocumentField
		confidence     float64
		expectedOutput string
	}{
		{
			name: "Single item with high confidence",
			documentFields: map[string]diDocumentField{
				"Items": {
					ValueArray: &[]diDocumentFieldItem{
						{
							ValueObject: map[string]diDocumentField{
								"Description": {Content: "Test item description"},
							},
						},
					},
				},
				"InvoiceTotal": {Confidence: 0.95},
			},
			confidence:     0.95,
			expectedOutput: "- Test item description\n\nInvoiceTotal confidence: 0.95",
		},
		{
			name: "Multiple items with long description",
			documentFields: map[string]diDocumentField{
				"Items": {
					ValueArray: &[]diDocumentFieldItem{
						{
							ValueObject: map[string]diDocumentField{
								"Description": {Content: "This description is longer than forty characters"},
							},
						},
						{
							ValueObject: map[string]diDocumentField{
								"Description": {Content: "Another very long description of an item, product, or whatever"},
							},
						},
					},
				},
				"InvoiceTotal": {Confidence: 0.85},
			},
			confidence: 0.85,
			expectedOutput: "- This description is longer than forty characters\n" +
				"- Another very long description of an item, product, or whatever\n\nInvoiceTotal confidence: 0.85",
		},
		{
			name: "Empty items array",
			documentFields: map[string]diDocumentField{
				"Items":        {ValueArray: &[]diDocumentFieldItem{}},
				"InvoiceTotal": {Confidence: 1.0},
			},
			confidence:     1.0,
			expectedOutput: "\n\nInvoiceTotal confidence: 1.00",
		},
		{
			name: "Nil gross confidence",
			documentFields: map[string]diDocumentField{
				"Items": {
					ValueArray: &[]diDocumentFieldItem{
						{
							ValueObject: map[string]diDocumentField{
								"Description": {Content: "Item without confidence"},
							},
						},
					},
				},
			},
			confidence:     0.0,
			expectedOutput: "- Item without confidence\n\nInvoiceTotal confidence: -",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := diDocument{
				Fields:     tt.documentFields,
				Confidence: tt.confidence,
			}
			c := d.createComment()

			require.Equal(t, tt.expectedOutput, c)
		})
	}
}
func Test_diDocument_createInvoiceName(t *testing.T) {
	tests := []struct {
		name           string
		documentFields map[string]diDocumentField
		expectedOutput string
	}{
		{
			name: "Single item with description",
			documentFields: map[string]diDocumentField{
				"VendorName": {Content: "Vendor A"},
				"Items": {
					ValueArray: &[]diDocumentFieldItem{
						{ValueObject: map[string]diDocumentField{"Description": {Content: "Item with short description"}}},
					},
				},
			},
			expectedOutput: "Item with short description from Vendor A to ",
		},
		{
			name: "Single item with truncated description",
			documentFields: map[string]diDocumentField{
				"VendorName": {Content: "Vendor B"},
				"Items": {
					ValueArray: &[]diDocumentFieldItem{
						{
							ValueObject: map[string]diDocumentField{
								"Description": {Content: "Very long description exceeding forty characters for truncation"},
							},
						},
					},
				},
			},
			expectedOutput: "Very long description exceeding forty... from Vendor B to ",
		},
		{
			name: "Multiple items",
			documentFields: map[string]diDocumentField{
				"VendorName":   {Content: "Vendor C"},
				"CustomerName": {Content: "Customer X"},
				"InvoiceId":    {Content: "INV12345"},
				"Items": {
					ValueArray: &[]diDocumentFieldItem{
						{
							ValueObject: map[string]diDocumentField{
								"Description": {Content: "First item"},
							},
						},
						{
							ValueObject: map[string]diDocumentField{
								"Description": {Content: "Second item"},
							},
						},
					},
				},
			},
			expectedOutput: "Invoice INV12345 from Vendor C to Customer X",
		},
		{
			name: "Missing VendorName field",
			documentFields: map[string]diDocumentField{
				"Items": {
					ValueArray: &[]diDocumentFieldItem{
						{
							ValueObject: map[string]diDocumentField{
								"Description": {Content: "Single item description"},
							},
						},
					},
				},
			},
			expectedOutput: "Single item description from  to ",
		},
		{
			name: "Empty Items array",
			documentFields: map[string]diDocumentField{
				"VendorName":   {Content: "Vendor D"},
				"CustomerName": {Content: "Customer Y"},
				"InvoiceId":    {Content: "INV67890"},
				"Items":        {ValueArray: &[]diDocumentFieldItem{}},
			},
			expectedOutput: "Invoice INV67890 from Vendor D to Customer Y",
		},
		{
			name: "Nil Items array",
			documentFields: map[string]diDocumentField{
				"VendorName":   {Content: "Vendor E"},
				"CustomerName": {Content: "Customer Z"},
				"InvoiceId":    {Content: "INV99999"},
			},
			expectedOutput: "Invoice INV99999 from Vendor E to Customer Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := diDocument{
				Fields: tt.documentFields,
			}
			require.Equal(t, tt.expectedOutput, d.createInvoiceName())
		})
	}
}

func Test_diDocument_getContentFieldCommaSeperated(t *testing.T) {
	tests := []struct {
		name           string
		documentFields map[string]diDocumentField
		fieldName      string
		expectedOutput string
	}{
		{
			name: "Single line content",
			documentFields: map[string]diDocumentField{
				"ExampleField": {Content: "Single line data"},
			},
			fieldName:      "ExampleField",
			expectedOutput: "Single line data",
		},
		{
			name: "Multiline content",
			documentFields: map[string]diDocumentField{
				"ExampleField": {Content: "Line 1\nLine 2\nLine 3"},
			},
			fieldName:      "ExampleField",
			expectedOutput: "Line 1, Line 2, Line 3",
		},
		{
			name: "Content with trailing newline",
			documentFields: map[string]diDocumentField{
				"ExampleField": {Content: "Line 1\n"},
			},
			fieldName:      "ExampleField",
			expectedOutput: "Line 1",
		},
		{
			name: "Empty content",
			documentFields: map[string]diDocumentField{
				"ExampleField": {Content: ""},
			},
			fieldName:      "ExampleField",
			expectedOutput: "",
		},
		{
			name: "Field not found",
			documentFields: map[string]diDocumentField{
				"AnotherField": {Content: "Sample data"},
			},
			fieldName:      "MissingField",
			expectedOutput: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diDoc := diDocument{Fields: tt.documentFields}
			require.Equal(t, tt.expectedOutput, diDoc.getContentFieldCommaSeperated(tt.fieldName))
		})
	}
}

func Test_diDocument_getGross(t *testing.T) {
	tests := []struct {
		name           string
		documentFields map[string]diDocumentField
		expectedGross  *float64
	}{
		{
			name: "Valid gross amount",
			documentFields: map[string]diDocumentField{
				"InvoiceTotal": {ValueCurrency: &diCurrency{Amount: 123.45}},
			},
			expectedGross: func() *float64 { v := 123.45; return &v }(),
		},
		{
			name:           "Missing InvoiceTotal field",
			documentFields: map[string]diDocumentField{},
			expectedGross:  nil,
		},
		{
			name: "InvoiceTotal present with zero amount",
			documentFields: map[string]diDocumentField{
				"InvoiceTotal": {ValueCurrency: &diCurrency{Amount: 0.0}},
			},
			expectedGross: func() *float64 { v := 0.0; return &v }(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diDoc := diDocument{Fields: tt.documentFields}
			require.Equal(t, tt.expectedGross, diDoc.getGross())
		})
	}
}

func Test_diDocument_getGrossConfidence(t *testing.T) {
	tests := []struct {
		name           string
		documentFields map[string]diDocumentField
		expectedOutput *float64
	}{
		{
			name: "Field exists with confidence",
			documentFields: map[string]diDocumentField{
				"InvoiceTotal": {Confidence: 0.85},
			},
			expectedOutput: func() *float64 { v := 0.85; return &v }(),
		},
		{
			name:           "Field missing",
			documentFields: map[string]diDocumentField{},
			expectedOutput: nil,
		},
		{
			name: "Field exists with zero confidence",
			documentFields: map[string]diDocumentField{
				"InvoiceTotal": {Confidence: 0.0},
			},
			expectedOutput: func() *float64 { v := 0.0; return &v }(),
		},
		{
			name: "Confidence not set in field",
			documentFields: map[string]diDocumentField{
				"InvoiceTotal": {},
			},
			expectedOutput: func() *float64 { v := 0.0; return &v }(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diDoc := diDocument{Fields: tt.documentFields}
			require.Equal(t, tt.expectedOutput, diDoc.getGrossConfidence())
		})
	}
}

func Test_diDocument_getVat(t *testing.T) {
	tests := []struct {
		name           string
		documentFields map[string]diDocumentField
		expectedVat    *float64
	}{
		{
			name: "Valid VAT field",
			documentFields: map[string]diDocumentField{
				"TaxDetails": {
					ValueArray: &[]diDocumentFieldItem{
						{
							ValueObject: map[string]diDocumentField{
								"Rate": {Content: "10%"},
							},
						},
					},
				},
			},
			expectedVat: func() *float64 { v := 10.0; return &v }(),
		},
		{
			name: "Invalid VAT field format",
			documentFields: map[string]diDocumentField{
				"TaxDetails": {
					ValueArray: &[]diDocumentFieldItem{
						{
							ValueObject: map[string]diDocumentField{
								"Rate": {Content: "Invalid%"},
							},
						},
					},
				},
			},
			expectedVat: nil,
		},
		{
			name: "Multiple TaxDetails entries",
			documentFields: map[string]diDocumentField{
				"TaxDetails": {
					ValueArray: &[]diDocumentFieldItem{
						{
							ValueObject: map[string]diDocumentField{
								"Rate": {Content: "10%"},
							},
						},
						{
							ValueObject: map[string]diDocumentField{
								"Rate": {Content: "20%"},
							},
						},
					},
				},
			},
			expectedVat: nil,
		},
		{
			name:           "Missing TaxDetails field",
			documentFields: map[string]diDocumentField{},
			expectedVat:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diDoc := diDocument{Fields: tt.documentFields}
			require.Equal(t, tt.expectedVat, diDoc.getVat())
		})
	}
}

func Test_diDocument_isTypeInvoice(t *testing.T) {
	diDoc := diDocument{DocType: documentTypeInvoice}
	require.True(t, diDoc.isTypeInvoice())
}

func Test_diDocument_isNotTypeInvoice(t *testing.T) {
	diDoc := diDocument{}
	require.False(t, diDoc.isTypeInvoice())
}
