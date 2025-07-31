# Go Legacy Service

This is a legacy service that does not subscribe to API evolution events.
It should never be automatically triggered when the API definitions change.

This service exists to verify that the selective fan-out only affects subscribed services.