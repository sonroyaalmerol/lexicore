package ad

import (
	"fmt"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

func (o *ADOperator) buildDN(id source.Identity, userBaseDN string) string {
	cn := id.DisplayName
	if cn == "" {
		cn = id.Username
	}
	return fmt.Sprintf("CN=%s,%s", cn, userBaseDN)
}
