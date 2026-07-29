local surface = game.surfaces[surface_name]
local revived = 0
local ghosts = surface.find_entities_filtered({type = "entity-ghost"})
for _, ghost in pairs(ghosts) do
  local _, entity = ghost.revive({raise_revive = false})
  assert(entity, "failed to revive deferred ghost")
  revived = revived + 1
end
rcon.print(
  "revived=" .. revived
    .. " remaining="
    .. #surface.find_entities_filtered({type = "entity-ghost"})
)
