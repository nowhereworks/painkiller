package q16

default allow := false

allow if {
	input.action == "read"
	some record in data.records
	record.id == input.resource
	record.tenant == input.tenant
}
