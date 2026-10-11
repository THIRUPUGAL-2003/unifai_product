package gateway

import "testing"

func TestResizePoolsShrinksAndGrows(t *testing.T) {
	gateway := &Gateway{}
	gateway.resizePools(4)
	if gateway.prewarmedPool != 4 {
		t.Fatalf("prewarmed = %d", gateway.prewarmedPool)
	}
	gateway.resizePools(2)
	if gateway.prewarmedPool != 2 {
		t.Fatalf("shrunk prewarmed = %d", gateway.prewarmedPool)
	}
	req := gateway.getGatewayRequest()
	if req == nil {
		t.Fatal("expected a pooled request")
	}
	gateway.releaseGatewayRequest(req)
	gateway.resizePools(3)
	if gateway.prewarmedPool != 3 {
		t.Fatalf("grown prewarmed = %d", gateway.prewarmedPool)
	}
}
