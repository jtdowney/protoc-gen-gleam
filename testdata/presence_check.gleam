// Run by TestPresenceFieldsEncode with the generated module alongside.
import example/proto/presence.{Marker, Presence, PresenceValueCount, PresenceValueText}

pub fn main() -> Nil {
  let set_to_zero =
    Presence(
      note: Ok(""),
      delta: Ok(0),
      marker: Ok(Marker),
      value: PresenceValueCount(0),
    )
  let assert <<0x0A, 0, 0x10, 0, 0x1A, 0, 0x28, 0>> =
    presence.encode_presence(set_to_zero)
  let assert Ok(decoded) =
    presence.decode_presence(presence.encode_presence(set_to_zero))
  let assert True = decoded == set_to_zero

  let unset =
    Presence(note: Error(Nil), delta: Error(Nil), marker: Error(Nil), value: PresenceValueText(""))
  let assert <<0x22, 0>> = presence.encode_presence(unset)
  let assert Ok(decoded) = presence.decode_presence(presence.encode_presence(unset))
  let assert True = decoded == unset
  Nil
}
