"""The messages and enumerations of gitd's contract that this client uses."""

from .contract import Contract

contract = Contract.load()

InfoRequest = contract.message("InfoRequest")
InfoResponse = contract.message("InfoResponse")
ProbeRequest = contract.message("ProbeRequest")
ProbeResponse = contract.message("ProbeResponse")
Revision = contract.message("Revision")
PrepareRequest = contract.message("PrepareRequest")
PrepareResponse = contract.message("PrepareResponse")
UpdateRequest = contract.message("UpdateRequest")
UpdateResponse = contract.message("UpdateResponse")
CheckoutRequest = contract.message("CheckoutRequest")
CheckoutResponse = contract.message("CheckoutResponse")
DescribeRequest = contract.message("DescribeRequest")
DescribeResponse = contract.message("DescribeResponse")
PublishRequest = contract.message("PublishRequest")
PublishResponse = contract.message("PublishResponse")
RemoveRequest = contract.message("RemoveRequest")
RemoveResponse = contract.message("RemoveResponse")
AuditRequest = contract.message("AuditRequest")
AuditResponse = contract.message("AuditResponse")
GitFailure = contract.message("GitFailure")

STRATEGY = contract.enumeration("Strategy")
ACCESS = contract.enumeration("ProbeResponse.Access")
REASON = contract.enumeration("GitFailure.Reason")
