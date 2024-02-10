package GeoHash

type GeoService interface {
	GeoAdd(Points) (bool, error)
	GeoHash(Points) (string, error)
	GeoDistance(Points, Points) (error, float64)
	GeoPosition(string) ([]Points, error)
	GeoDel(string) (bool, error)
}

type Points struct {
	Longitude float64
	Latitude  float64
}
