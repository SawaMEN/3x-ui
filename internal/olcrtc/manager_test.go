package olcrtc

import (
 "strings"
 "testing"
)

func TestNormalizeJitsiDefaults(t *testing.T) {
 input:=ClientInput{ClientID:"example"}
 if err:=normalize(&input);err!=nil {t.Fatal(err)}
 if input.Carrier!="jitsi" || input.Transport!="datachannel" {t.Fatalf("unexpected defaults: %+v",input)}
 if !strings.HasPrefix(input.RoomID,"https://meet.handyweb.org/olcrtc-") {t.Fatalf("unexpected room: %s",input.RoomID)}
 if len(input.Key)!=64 {t.Fatalf("key should contain 64 hex chars, got %d",len(input.Key))}
 if input.DNS!="1.1.1.1:53" {t.Fatalf("unexpected DNS: %s",input.DNS)}
}
func TestNormalizeRejectsUnsupportedCombinations(t *testing.T) {
 for _,tc:=range []struct{carrier,transport string}{
  {"telemost","datachannel"},
  {"telemost","seichannel"},
  {"wbstream","datachannel"},
  {"other","datachannel"},
 } {
  t.Run(tc.carrier+"_"+tc.transport,func(t *testing.T){
   input:=ClientInput{ClientID:"test",Carrier:tc.carrier,Transport:tc.transport,RoomID:"room"}
   if err:=normalize(&input);err==nil {t.Fatalf("expected error for %+v",input)}
  })
 }
}
func TestNormalizeRequiresExternalRoom(t *testing.T) {
 for _,provider:=range []string{"telemost","wbstream"}{
  input:=ClientInput{ClientID:"test",Carrier:provider}
  if err:=normalize(&input);err==nil {t.Fatalf("%s must require existing room",provider)}
 }
}
func TestNormalizeRejectsBadInputs(t *testing.T) {
 for name,input:=range map[string]ClientInput{
  "unsafe client":{ClientID:"../escape",Carrier:"jitsi"},
  "insecure room":{ClientID:"a",Carrier:"jitsi",RoomID:"http://meet.test/room"},
  "incorrect key":{ClientID:"a",Carrier:"jitsi",Key:"abc"},
  "invalid DNS":{ClientID:"a",Carrier:"jitsi",DNS:"1.1.1.1:70000"},
 } {
  t.Run(name,func(t *testing.T){
   if err:=normalize(&input);err==nil {t.Fatalf("expected error for %s",name)}
  })
 }
}
func TestClientLink(t *testing.T) {
 var client Client
 client.ID="user-1234"
 var location Location
 location.Carrier="jitsi"
 location.Transport.Type="datachannel"
 location.Endpoint.RoomID="https://meet.handyweb.org/test"
 location.Endpoint.Key=strings.Repeat("a",64)
 client.Locations=[]Location{location}
 link:=ClientLink(client)
 expected:="olcrtc://jitsi?datachannel@https://meet.handyweb.org/test#"+strings.Repeat("a",64)+"$user-1234"
 if link!=expected {t.Fatalf("got %q, want %q",link,expected)}
}
