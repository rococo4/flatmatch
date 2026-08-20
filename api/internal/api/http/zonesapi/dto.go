package zonesapi

import "flatmatch/api/internal/service/pools"

type topZoneDTO struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	NameIn string `json:"name_in"`
	People int    `json:"people"`
}

type topResponse struct {
	Zones []topZoneDTO `json:"zones"`
}

func topZones(top []pools.ZoneTop) []topZoneDTO {
	out := make([]topZoneDTO, 0, len(top))
	for _, z := range top {
		out = append(out, topZoneDTO{
			ID:     z.Zone.ID,
			Name:   z.Zone.Name,
			NameIn: z.Zone.NameIn,
			People: z.People,
		})
	}
	return out
}
