package login

import "wonderland-gonline/internal/localization"

import "strconv"

// disconnectReasons are the texts the action-0 handler (0x2de0b0) stores
// for each subcommand before the server closes the connection. Subcommand
// 39 and codes past the table use the default.
var disconnectReasons = map[byte]string{
	1: localization.UI("ui.login.too_many_packets_11c9f4d6"), 2: localization.UI("ui.login.fail_q_a_3_times_c8c445a4"), 3: localization.UI("ui.login.wrong_pwd_9d2ff996"), 4: localization.UI("ui.login.disconnected_04dfac36"),
	5: localization.UI("ui.login.illegal_activity_1_fb568afc"), 6: localization.UI("ui.login.illegal_activity_2_7cccac07"), 7: localization.UI("ui.login.event_error_abe4d255"), 8: localization.UI("ui.login.incorrect_trigger_197fc426"),
	9: localization.UI("ui.login.wrong_event_table_dfa7d4f4"), 10: localization.UI("ui.login.table_error_2bd02264"), 11: localization.UI("ui.login.limiter_tiggered_a8c16a7b"), 12: localization.UI("ui.login.illegal_press_8cecbcc5"),
	13: localization.UI("ui.login.stream_violation_a213930d"), 14: localization.UI("ui.login.movement_too_fast_a22b38ef"), 15: localization.UI("ui.login.delete_successful_c427d67e"), 16: localization.UI("ui.login.blocked_ip_detected_949eed2b"),
	17: localization.UI("ui.login.update_game_files_9f33b009"), 18: localization.UI("ui.login.data_altered_1152fc3a"), 19: localization.UI("ui.login.character_is_already_logged_in_f6b144f1"), 20: localization.UI("ui.login.abnormal_d_c_cf7092d3"),
	21: localization.UI("ui.login.abnormal_safe_file_data_6ca8f84a"), 22: localization.UI("ui.login.invalid_packet_data_4d54aaff"), 23: localization.UI("ui.login.name_changed_2a9dd471"), 24: localization.UI("ui.login.password_too_short_d49b5265"),
	25: localization.UI("ui.login.duplicated_name_f05452e2"), 26: localization.UI("ui.login.event_trigger_error_290d2a5d"), 27: localization.UI("ui.login.login_error_d_c_55ca49e8"), 28: localization.UI("ui.login.firewall_d_c_a7f559f1"),
	29: localization.UI("ui.login.too_much_data_3542d97f"), 30: localization.UI("ui.login.account_lock_f7b5bf24"), 31: localization.UI("ui.login.login_id_unavailable_0559fa05"), 32: localization.UI("ui.login.battle_error_ce2304d4"),
	33: localization.UI("ui.login.scene_error_1a1176dc"), 34: localization.UI("ui.login.other_server_login_a35eda9e"), 35: localization.UI("ui.login.confirm_licence_cc310199"), 36: localization.UI("ui.login.invalid_id_ea6bde75"),
	37: localization.UI("ui.login.scene_changed_e2c03ae2"), 38: localization.UI("ui.login.wrong_target_scene_53a5a8e9"), 40: localization.UI("ui.login.packet_manipulation_bb7f2fa4"), 41: localization.UI("ui.login.relogin_top_up_5775e673"),
	42: localization.UI("ui.login.battle_error_ce2304d4"), 43: localization.UI("ui.login.battle_error_0_7c89858d"), 44: localization.UI("ui.login.wrong_tigger_scene_a67cd46a"), 45: localization.UI("ui.login.account_banned_afcd26ab"),
	46: localization.UI("ui.login.q_a_cheater_detected_16c8ff4d"), 47: localization.UI("ui.login.battle_end_error_78732ae8"), 48: localization.UI("ui.login.use_illegal_skill_9c8bcb6f"), 49: localization.UI("ui.login.under_18_y_o_curfew_c012393b"),
	50: localization.UI("ui.login.log_into_pvp_server_81abe6fa"), 51: localization.UI("ui.login.can_t_login_pvp_server_d248df5f"), 52: localization.UI("ui.login.can_t_login_pvp_server_d248df5f"), 53: localization.UI("ui.login.server_not_open_yet_7ffb2111"),
	54: localization.UI("ui.login.left_pvp_event_fc787ed0"), 55: localization.UI("ui.login.no_transport_on_world_map_042c687a"), 56: localization.UI("ui.login.password_too_long_73d26e19"), 57: localization.UI("ui.login.pwd_del_pwd_match_e50a511d"),
	58: localization.UI("ui.login.beta_access_error_b5e9732b"), 59: localization.UI("ui.login.relay_server_d_c_53852ddc"), 60: localization.UI("ui.login.connection_lost_6c44751e"), 61: localization.UI("ui.login.server_is_busy_a38b0f75"),
	62: localization.UI("ui.login.incorrect_password_0c8b937a"), 63: localization.UI("ui.login.mini_game_error_d_c_e3f8b357"), 64: localization.UI("ui.login.illegal_app_used_3d306c62"), 65: localization.UI("ui.login.wrong_version_29357566"),
	66: localization.UI("ui.login.ip_zone_blocked_54877da4"), 67: localization.UI("ui.login.external_d_c_3bd04606"), 68: localization.UI("ui.login.ip_address_error_e838a52b"), 69: localization.UI("ui.login.battle_protocol_error_2372e18c"),
	70: localization.UI("ui.login.firewall_triggered_f441f286"), 71: localization.UI("ui.login.teleport_exploit_49eec55a"), 72: localization.UI("ui.login.item_dat_file_error_46c3c90d"), 73: localization.UI("ui.login.instance_map_purged_60388c0c"),
	74: localization.UI("ui.login.interserver_d_c_bed7afd7"), 75: localization.UI("ui.login.max_logins_reached_c8113292"), 76: localization.UI("ui.login.event_violation_3951f8dd"), 77: localization.UI("ui.login.login_error_10_times_584a6bca"),
	78: localization.UI("ui.login.ip_blocked_df6be4a9"), 79: localization.UI("ui.login.routing_error_wait_42458e14"), 80: localization.UI("ui.login.acct_locked_ccf1de4b"), 81: localization.UI("ui.login.log_in_again_later_fcbb0403"),
	// Big5 "請使用自定義帳號登入" ("log in with a custom account").
	82: localization.UI("ui.login.custom_account"),
}

const (
	defaultReason = "ui.login.connection_lost_6c44751e" // 0x2eff88
	defaultCode   = 0xff
)

// namedReasons is the bit set at 0x2f01e4: codes shown with their text.
// Others read "D/c:" and the code.
var namedReasons = [32]byte{0x08, 0x80, 0x0a, 0x80, 0x00, 0x22, 0x68, 0xf4, 0x1f, 0xfa, 0x07,
	31: 0x80}

// DisconnectReason is the text the action-0 handler leaves for the
// disconnect message (DAT_00828128 +0x18). gameState is
// PTR_DAT_004caa18 +4; states 5..16 always show the text.
func DisconnectReason(sub byte, gameState int) []byte {
	text, ok := disconnectReasons[sub]
	code := int(sub)
	if !ok {
		text, code = localization.UI(defaultReason), defaultCode
	}
	if uint(gameState-5) < 12 || namedReasons[code/8]&(1<<(code%8)) != 0 {
		return []byte(text + ":" + strconv.Itoa(code))
	}
	return []byte("D/c:" + strconv.Itoa(code))
}

func init() {
	localization.Register(func() {
		disconnectReasons = map[byte]string{
			1: localization.UI("ui.login.too_many_packets_11c9f4d6"), 2: localization.UI("ui.login.fail_q_a_3_times_c8c445a4"), 3: localization.UI("ui.login.wrong_pwd_9d2ff996"), 4: localization.UI("ui.login.disconnected_04dfac36"),
			5: localization.UI("ui.login.illegal_activity_1_fb568afc"), 6: localization.UI("ui.login.illegal_activity_2_7cccac07"), 7: localization.UI("ui.login.event_error_abe4d255"), 8: localization.UI("ui.login.incorrect_trigger_197fc426"),
			9: localization.UI("ui.login.wrong_event_table_dfa7d4f4"), 10: localization.UI("ui.login.table_error_2bd02264"), 11: localization.UI("ui.login.limiter_tiggered_a8c16a7b"), 12: localization.UI("ui.login.illegal_press_8cecbcc5"),
			13: localization.UI("ui.login.stream_violation_a213930d"), 14: localization.UI("ui.login.movement_too_fast_a22b38ef"), 15: localization.UI("ui.login.delete_successful_c427d67e"), 16: localization.UI("ui.login.blocked_ip_detected_949eed2b"),
			17: localization.UI("ui.login.update_game_files_9f33b009"), 18: localization.UI("ui.login.data_altered_1152fc3a"), 19: localization.UI("ui.login.character_is_already_logged_in_f6b144f1"), 20: localization.UI("ui.login.abnormal_d_c_cf7092d3"),
			21: localization.UI("ui.login.abnormal_safe_file_data_6ca8f84a"), 22: localization.UI("ui.login.invalid_packet_data_4d54aaff"), 23: localization.UI("ui.login.name_changed_2a9dd471"), 24: localization.UI("ui.login.password_too_short_d49b5265"),
			25: localization.UI("ui.login.duplicated_name_f05452e2"), 26: localization.UI("ui.login.event_trigger_error_290d2a5d"), 27: localization.UI("ui.login.login_error_d_c_55ca49e8"), 28: localization.UI("ui.login.firewall_d_c_a7f559f1"),
			29: localization.UI("ui.login.too_much_data_3542d97f"), 30: localization.UI("ui.login.account_lock_f7b5bf24"), 31: localization.UI("ui.login.login_id_unavailable_0559fa05"), 32: localization.UI("ui.login.battle_error_ce2304d4"),
			33: localization.UI("ui.login.scene_error_1a1176dc"), 34: localization.UI("ui.login.other_server_login_a35eda9e"), 35: localization.UI("ui.login.confirm_licence_cc310199"), 36: localization.UI("ui.login.invalid_id_ea6bde75"),
			37: localization.UI("ui.login.scene_changed_e2c03ae2"), 38: localization.UI("ui.login.wrong_target_scene_53a5a8e9"), 40: localization.UI("ui.login.packet_manipulation_bb7f2fa4"), 41: localization.UI("ui.login.relogin_top_up_5775e673"),
			42: localization.UI("ui.login.battle_error_ce2304d4"), 43: localization.UI("ui.login.battle_error_0_7c89858d"), 44: localization.UI("ui.login.wrong_tigger_scene_a67cd46a"), 45: localization.UI("ui.login.account_banned_afcd26ab"),
			46: localization.UI("ui.login.q_a_cheater_detected_16c8ff4d"), 47: localization.UI("ui.login.battle_end_error_78732ae8"), 48: localization.UI("ui.login.use_illegal_skill_9c8bcb6f"), 49: localization.UI("ui.login.under_18_y_o_curfew_c012393b"),
			50: localization.UI("ui.login.log_into_pvp_server_81abe6fa"), 51: localization.UI("ui.login.can_t_login_pvp_server_d248df5f"), 52: localization.UI("ui.login.can_t_login_pvp_server_d248df5f"), 53: localization.UI("ui.login.server_not_open_yet_7ffb2111"),
			54: localization.UI("ui.login.left_pvp_event_fc787ed0"), 55: localization.UI("ui.login.no_transport_on_world_map_042c687a"), 56: localization.UI("ui.login.password_too_long_73d26e19"), 57: localization.UI("ui.login.pwd_del_pwd_match_e50a511d"),
			58: localization.UI("ui.login.beta_access_error_b5e9732b"), 59: localization.UI("ui.login.relay_server_d_c_53852ddc"), 60: localization.UI("ui.login.connection_lost_6c44751e"), 61: localization.UI("ui.login.server_is_busy_a38b0f75"),
			62: localization.UI("ui.login.incorrect_password_0c8b937a"), 63: localization.UI("ui.login.mini_game_error_d_c_e3f8b357"), 64: localization.UI("ui.login.illegal_app_used_3d306c62"), 65: localization.UI("ui.login.wrong_version_29357566"),
			66: localization.UI("ui.login.ip_zone_blocked_54877da4"), 67: localization.UI("ui.login.external_d_c_3bd04606"), 68: localization.UI("ui.login.ip_address_error_e838a52b"), 69: localization.UI("ui.login.battle_protocol_error_2372e18c"),
			70: localization.UI("ui.login.firewall_triggered_f441f286"), 71: localization.UI("ui.login.teleport_exploit_49eec55a"), 72: localization.UI("ui.login.item_dat_file_error_46c3c90d"), 73: localization.UI("ui.login.instance_map_purged_60388c0c"),
			74: localization.UI("ui.login.interserver_d_c_bed7afd7"), 75: localization.UI("ui.login.max_logins_reached_c8113292"), 76: localization.UI("ui.login.event_violation_3951f8dd"), 77: localization.UI("ui.login.login_error_10_times_584a6bca"),
			78: localization.UI("ui.login.ip_blocked_df6be4a9"), 79: localization.UI("ui.login.routing_error_wait_42458e14"), 80: localization.UI("ui.login.acct_locked_ccf1de4b"), 81: localization.UI("ui.login.log_in_again_later_fcbb0403"),
			// Big5 "請使用自定義帳號登入" ("log in with a custom account").
			82: localization.UI("ui.login.custom_account"),
		}
	})
}
