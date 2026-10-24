package app.pwhs.blockads.data.datastore.prefs

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

class TailscalePreferences(private val dataStore: DataStore<Preferences>) {

    companion object {
        val KEY_ENABLED = booleanPreferencesKey("tailscale_enabled")
        val KEY_AUTH_KEY = stringPreferencesKey("tailscale_auth_key")
        val KEY_HOSTNAME = stringPreferencesKey("tailscale_hostname")
        val KEY_ALLOWED_CIDRS = stringPreferencesKey("tailscale_allowed_cidrs")
        val KEY_CONTROL_URL = stringPreferencesKey("tailscale_control_url")

        const val DEFAULT_HOSTNAME = "blockads-android"
    }

    val enabled: Flow<Boolean> = dataStore.data.map { prefs ->
        prefs[KEY_ENABLED] ?: false
    }

    val authKey: Flow<String> = dataStore.data.map { prefs ->
        prefs[KEY_AUTH_KEY] ?: ""
    }

    val hostname: Flow<String> = dataStore.data.map { prefs ->
        prefs[KEY_HOSTNAME] ?: DEFAULT_HOSTNAME
    }

    val allowedCIDRs: Flow<String> = dataStore.data.map { prefs ->
        prefs[KEY_ALLOWED_CIDRS] ?: ""
    }

    val controlURL: Flow<String> = dataStore.data.map { prefs ->
        prefs[KEY_CONTROL_URL] ?: ""
    }

    class Snapshot(
        val enabled: Boolean,
        val authKey: String,
        val hostname: String,
        val allowedCIDRs: String,
        val controlURL: String,
    ) {
        override fun toString(): String =
            "Snapshot(enabled=$enabled, hostname=$hostname, allowedCIDRs=$allowedCIDRs, controlURL=$controlURL)"
    }

    suspend fun snapshot(): Snapshot = Snapshot(
        enabled = enabled.first(),
        authKey = authKey.first(),
        hostname = hostname.first(),
        allowedCIDRs = allowedCIDRs.first(),
        controlURL = controlURL.first()
    )

    suspend fun setEnabled(value: Boolean) {
        dataStore.edit { it[KEY_ENABLED] = value }
    }

    suspend fun setAuthKey(value: String) {
        dataStore.edit { it[KEY_AUTH_KEY] = value }
    }

    suspend fun setHostname(value: String) {
        dataStore.edit { it[KEY_HOSTNAME] = value }
    }

    suspend fun setAllowedCIDRs(value: String) {
        dataStore.edit { it[KEY_ALLOWED_CIDRS] = value }
    }

    suspend fun setControlURL(value: String) {
        dataStore.edit { it[KEY_CONTROL_URL] = value }
    }
}
