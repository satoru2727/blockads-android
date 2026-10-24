package app.pwhs.blockads.service.vpn

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class TailscaleConfigJsonTest {

    @Test
    fun `disabled configuration is empty`() {
        val json = TailscaleConfigJson.build(input(enabled = false))

        assertEquals("", json)
    }

    @Test
    fun `build serializes trimmed CIDR list for JSON`() {
        val json = TailscaleConfigJson.build(
            input(allowedCIDRs = "10.0.0.0/8,  fd00::/8,,")
        )
        val parsed = JSONObject(json)

        assertEquals("10.0.0.0/8", parsed.getJSONArray("allowedCIDRs").getString(0))
        assertEquals("fd00::/8", parsed.getJSONArray("allowedCIDRs").getString(1))
    }

    @Test
    fun `state directory is rooted below app files directory`() {
        assertEquals(
            "/data/user/0/app/files/tailscale",
            TailscaleConfigJson.stateDir(File("/data/user/0/app/files"))
        )
    }

    @Test
    fun `auth key is not present in input toString`() {
        val input = input(authKey = "tskey-secret")

        assertFalse(input.toString().contains("tskey-secret"))
        assertTrue(TailscaleConfigJson.build(input).contains("tskey-secret"))
    }

    private fun input(
        enabled: Boolean = true,
        stateDir: String = "/data/user/0/app/files/tailscale",
        hostname: String = "blockads-android",
        authKey: String = "tskey-test",
        allowedCIDRs: String = "10.0.0.0/8",
        controlURL: String = "",
    ) = TailscaleConfigJson.Input(
        enabled = enabled,
        stateDir = stateDir,
        hostname = hostname,
        authKey = authKey,
        allowedCIDRs = allowedCIDRs,
        controlURL = controlURL
    )
}
