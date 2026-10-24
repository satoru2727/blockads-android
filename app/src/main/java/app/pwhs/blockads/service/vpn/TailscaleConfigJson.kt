package app.pwhs.blockads.service.vpn

import org.json.JSONArray
import org.json.JSONObject
import java.io.File

object TailscaleConfigJson {
    class Input(
        val enabled: Boolean,
        val stateDir: String,
        val hostname: String,
        val authKey: String,
        val allowedCIDRs: String,
        val controlURL: String,
    ) {
        override fun toString(): String =
            "Input(enabled=$enabled, stateDir=$stateDir, hostname=$hostname, " +
                "allowedCIDRs=$allowedCIDRs, controlURL=$controlURL)"
    }

    fun build(input: Input): String {
        if (!input.enabled) return ""

        val json = JSONObject()
            .put("stateDir", input.stateDir)
            .put("hostname", input.hostname)
            .put("authKey", input.authKey)
            .put("allowedCIDRs", input.allowedCIDRs.toCIDRArray())

        if (input.controlURL.isNotBlank()) {
            json.put("controlURL", input.controlURL.trim())
        }
        return json.toString()
    }

    fun stateDir(filesDir: File): String = File(filesDir, "tailscale").absolutePath

    fun hostname(json: String): String =
        runCatching { JSONObject(json).optString("hostname", "blockads-android") }
            .getOrDefault("blockads-android")

    private fun String.toCIDRArray(): JSONArray {
        val array = JSONArray()
        split(',')
            .map(String::trim)
            .filter(String::isNotEmpty)
            .forEach(array::put)
        return array
    }
}
