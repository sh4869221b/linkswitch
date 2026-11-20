package core

import (
	"errors"
	"strings"
)

const (
	appName            = "LinkSwitch"
	registeredAppsKey  = `Software\RegisteredApplications`
	startMenuParentKey = `Software\Clients\StartMenuInternet`
	clientKey          = startMenuParentKey + `\` + appName
	capabilitiesKey    = clientKey + `\Capabilities`
	protocolClassesKey = `Software\Classes`
	progIDHTTP         = "LinkSwitch.URL.HTTP"
	progIDHTTPS        = "LinkSwitch.URL.HTTPS"
	ownershipValue     = "LinkSwitchOwner"
	ownershipToken     = "LinkSwitch/14a5d3a0-4a04-447d-a15b-d601fd1209df"
)

var registrationTrees = [][2]string{
	{protocolClassesKey, progIDHTTP},
	{protocolClassesKey, progIDHTTPS},
	{startMenuParentKey, appName},
}

type registrationValue struct {
	path, name, value string
}

func browserRegistration(path string) ([]registrationValue, error) {
	if err := validateExecutablePath(path); err != nil {
		return nil, err
	}
	// Percent-prefixed tokens have special meaning in shell command
	// templates. Refuse an ambiguous installation path instead of guessing.
	if strings.ContainsRune(path, '%') {
		return nil, errors.New("LinkSwitch.exe を「%」を含まない場所に移動してから登録してください")
	}
	exe := `"` + path + `"`
	icon := exe + ",0"
	var values []registrationValue
	// Place markers first, so interrupted writes can be safely cleaned up.
	for _, entry := range registrationTrees {
		values = append(values, registrationValue{entry[0] + `\` + entry[1], ownershipValue, ownershipToken})
	}
	values = append(values,
		registrationValue{clientKey, "", appName},
		registrationValue{clientKey + `\DefaultIcon`, "", icon},
		registrationValue{clientKey + `\shell\open\command`, "", exe},
		registrationValue{capabilitiesKey, "ApplicationName", appName},
		registrationValue{capabilitiesKey, "ApplicationDescription", "リンクごとに開くブラウザーを選択します。"},
		registrationValue{capabilitiesKey, "ApplicationIcon", icon},
		registrationValue{capabilitiesKey + `\Startmenu`, "StartMenuInternet", appName},
		registrationValue{capabilitiesKey + `\URLAssociations`, "http", progIDHTTP},
		registrationValue{capabilitiesKey + `\URLAssociations`, "https", progIDHTTPS},
	)
	for _, id := range []string{progIDHTTP, progIDHTTPS} {
		key := protocolClassesKey + `\` + id
		values = append(values,
			registrationValue{key, "", "LinkSwitch URL"},
			registrationValue{key, "URL Protocol", ""},
			registrationValue{key + `\DefaultIcon`, "", icon},
			registrationValue{key + `\shell\open\command`, "", exe + ` --url "%1"`},
		)
	}
	return append(values, registrationValue{registeredAppsKey, appName, capabilitiesKey}), nil
}
