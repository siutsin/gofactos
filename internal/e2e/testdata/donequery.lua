local watch = storage.gofactos_e2e_done
assert(watch, "DONE monitor is not armed")
rcon.print(
  "seen=" .. tostring(watch.seen)
    .. " display=" .. watch.display
    .. " panels=" .. watch.panels
    .. " tick=" .. watch.tick
    .. " pulses=" .. watch.pulses
    .. " cadence=" .. tostring(watch.cadence)
    .. " bad_delta=" .. watch.bad_delta
)
