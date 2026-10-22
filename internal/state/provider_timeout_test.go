package state

import "testing"

func TestProviderFirstTokenDefaultAndPreservedOnLegacyWrite(t *testing.T) {
	s := newStore(t)
	p := provider("timeouts")
	p, err := s.PutProvider(p, 0, nil)
	if err != nil || p.FirstTokenTimeoutSeconds != 30 {
		t.Fatalf("%+v %v", p, err)
	}
	p.FirstTokenTimeoutSeconds = 72
	p, err = s.PutProvider(p, p.Revision, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.FirstTokenTimeoutSeconds = 0
	p, err = s.PutProvider(p, p.Revision, nil)
	if err != nil || p.FirstTokenTimeoutSeconds != 72 {
		t.Fatalf("legacy update reset value: %+v %v", p, err)
	}
	for _, invalid := range []int{-1, 3601} {
		p.FirstTokenTimeoutSeconds = invalid
		if p.Validate() == nil {
			t.Fatal("invalid timeout accepted")
		}
	}
}
